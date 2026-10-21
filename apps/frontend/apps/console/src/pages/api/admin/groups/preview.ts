/*
 * BFF de Sytue Admin — previsualización de una regla de grupo dinámico.
 *
 *   POST /api/admin/groups/preview  body { membership_rule }
 *     → { count, members[] }  (a quién incluiría la regla, SIN guardar)
 *
 * Proxya a Sytue ID (POST /api/v1/admin/groups/preview, Go) con el access token
 * httpOnly; exige rol admin/owner bajo RLS del tenant. La validación de la regla la
 * aplica Sytue ID (su 400 se propaga vía proxyStatus). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { coerceRule, previewDynamicMembers, proxyStatus } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['groups.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const body = (await ctx.request.json().catch(() => null)) as { membership_rule?: unknown } | null;
  const rule = coerceRule(body?.membership_rule);
  if (!rule || rule.conditions.length === 0)
    return json(
      { error: 'bad_request', message: 'La regla necesita al menos una condición.' },
      400,
    );
  try {
    return json(await previewDynamicMembers(guard.session, rule));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
