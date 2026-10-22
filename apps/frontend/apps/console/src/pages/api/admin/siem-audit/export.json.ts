/*
 * BFF de Sytue Admin — export JSON de la auditoría unificada (esquema SIEM).
 *
 *   GET /api/admin/siem-audit/export.json[?service=&action=&actor=&since=&until=&limit=]
 *       Descarga JSON (array de registros normalizados: ts, service, actor, action,
 *       target, detail) del conjunto agregado de la org activa. Apto para ingesta
 *       SIEM. Exige rol admin/owner.
 *
 * Igual que el export CSV: el conjunto se AGREGA en el BFF (fan-out) y se serializa
 * aquí, acotado por `limit`. El redirect de login es aceptable (navegación).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadUnifiedAudit, parseAuditFilters, toJson } from '../../../../lib/audit-siem.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guard;

  const filters = parseAuditFilters(new URL(ctx.request.url).searchParams);
  const { records } = await loadUnifiedAudit(guard.session, filters);
  const stamp = new Date().toISOString().slice(0, 10);
  return new Response(toJson(records), {
    status: 200,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'content-disposition': `attachment; filename="siem-audit-${stamp}.json"`,
      'cache-control': 'no-store',
    },
  });
};
