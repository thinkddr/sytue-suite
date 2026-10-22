/*
 * BFF de Sytue Admin — instancias de integración (Sytue Integration Gateway).
 *
 *   POST /api/admin/apps/instances   body { template_key, name, config? }   Crea una
 *        instancia (estado 'draft'); luego se le escriben secretos y se activa.
 *
 * Proxya al gateway (POST /api/v1/instances, Go) con el access token httpOnly; propaga
 * el status del servicio. Rol admin/owner. Puro passthrough (sin lógica).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { createInstance, proxyStatus } from '../../../../lib/gateway.js';

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
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const templateKey = typeof body?.template_key === 'string' ? body.template_key.trim() : '';
  const name = typeof body?.name === 'string' ? body.name.trim() : '';
  if (!templateKey || !name) {
    return json({ error: 'bad_request', message: 'template_key y name son obligatorios.' }, 400);
  }
  const config =
    body?.config && typeof body.config === 'object'
      ? (body.config as Record<string, string>)
      : undefined;
  try {
    return json(
      await createInstance(guard.session, {
        template_key: templateKey,
        name,
        ...(config ? { config } : {}),
      }),
      201,
    );
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
