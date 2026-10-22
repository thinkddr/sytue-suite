/*
 * BFF de Sytue Admin — llevarse LO PROPIO a Google Workspace o Microsoft 365.
 *
 *   POST /api/admin/migrations/self-out   { instance_id, domains }
 *
 * Sin personas: el gateway copia siempre el contenido de quien llama, leído con su
 * propia sesión de las mismas entregas que la copia de «Tus datos».
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { proxyStatus, selfOutbound, type MigrationDomain } from '../../../../lib/gateway.js';

export const prerender = false;

const OWN: readonly MigrationDomain[] = ['drive', 'contacts', 'mail', 'calendar'];

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);

  const body = (await ctx.request.json().catch(() => null)) as {
    instance_id?: unknown;
    domains?: unknown;
  } | null;
  const instanceId = typeof body?.instance_id === 'string' ? body.instance_id.trim() : '';
  const raw: unknown = body?.domains;
  const domains = Array.isArray(raw) ? OWN.filter((d) => (raw as unknown[]).includes(d)) : [];
  if (!instanceId || domains.length === 0) return json({ error: 'bad_request' }, 400);
  try {
    return json({ migrations: await selfOutbound(guard.session, instanceId, domains) }, 202);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
