/*
 * BFF de Sytue Admin — auditoría unificada (fan-out a todos los servicios).
 *
 *   GET /api/admin/siem-audit[?service=&action=&actor=&since=&until=&limit=]
 *       Conjunto agregado y normalizado (SIEM) de la org activa: { records, sources }.
 *
 * Fan-out best-effort a Sytue ID (identidad/admin), Mail y Drive (compliance) e
 * Integration Gateway (egress + decisiones de política) con el access token httpOnly
 * de la sesión; exige rol admin/owner. Cada servicio aísla por org (RLS). Si una
 * fuente cae, se reporta su estado en `sources` sin romper el resto.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadUnifiedAudit, parseAuditFilters } from '../../../lib/audit-siem.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const filters = parseAuditFilters(new URL(ctx.request.url).searchParams);
  const { records, sources } = await loadUnifiedAudit(guard.session, filters);
  return json({ records, sources });
};
