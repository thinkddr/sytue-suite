/*
 * BFF de Sytue Admin — export CSV de la auditoría unificada (esquema SIEM).
 *
 *   GET /api/admin/siem-audit/export.csv[?service=&action=&actor=&since=&until=&limit=]
 *       Descarga CSV del conjunto agregado y normalizado (ts, service, actor, action,
 *       target, detail) de la org activa. Exige rol admin/owner.
 *
 * El conjunto se AGREGA en el BFF (fan-out a todos los servicios) y se serializa aquí:
 * no es un stream de un solo backend, así que se materializa acotado por `limit` (el
 * fan-out ya topa por-fuente). El redirect de login es aceptable (navegación).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { loadUnifiedAudit, parseAuditFilters, toCsv } from '../../../../lib/audit-siem.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guard;

  const filters = parseAuditFilters(new URL(ctx.request.url).searchParams);
  const { records } = await loadUnifiedAudit(guard.session, filters);
  const stamp = new Date().toISOString().slice(0, 10);
  return new Response(toCsv(records), {
    status: 200,
    headers: {
      'content-type': 'text/csv; charset=utf-8',
      'content-disposition': `attachment; filename="siem-audit-${stamp}.csv"`,
      'cache-control': 'no-store',
    },
  });
};
