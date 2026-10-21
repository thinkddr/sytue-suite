/*
 * BFF de Sytue Admin — aplicaciones que inician sesión CON Sytue (Sytue ID como
 * proveedor OIDC).
 *
 *   GET  /api/admin/apps/oidc-clients   Lista las aplicaciones de la org activa (sin
 *        secreto).
 *   POST /api/admin/apps/oidc-clients   body { name, redirect_uris[],
 *        post_logout_redirect_uris?, scopes?, public? }   Registra una aplicación; la
 *        respuesta trae el client_secret UNA sola vez.
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/apps/oidc-clients, Go) con el access token
 * httpOnly; propaga el status del servicio (400/403/502). Rol admin/owner con
 * apps.manage. La validación real (URLs https exactas, scopes, PKCE) la aplica Sytue ID.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { createOidcApp, loadOidcApps, proxyStatus } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function guardStatus(s: number): Response {
  return json({ error: s === 403 ? 'forbidden' : 'unauthenticated' }, s === 403 ? 403 : 401);
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { data } = await loadOidcApps(guard.session);
  return json({ items: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const name = typeof body?.name === 'string' ? body.name.trim() : '';
  const list = (v: unknown): string[] =>
    Array.isArray(v)
      ? v
          .map(String)
          .map((s) => s.trim())
          .filter(Boolean)
      : [];
  const redirects = list(body?.redirect_uris);
  if (!name || redirects.length === 0) {
    return json({ error: 'bad_request', message: 'name y al menos una URL de redirección.' }, 400);
  }
  try {
    return json(
      await createOidcApp(guard.session, {
        name,
        redirect_uris: redirects,
        post_logout_redirect_uris: list(body?.post_logout_redirect_uris),
        scopes: list(body?.scopes),
        public: body?.public === true,
      }),
      201,
    );
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
