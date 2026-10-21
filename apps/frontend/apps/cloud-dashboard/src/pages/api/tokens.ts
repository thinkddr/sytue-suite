/*
 * BFF de tokens de API.
 *   POST { name, scope, project_id?, expires_at? } -> crea y devuelve el SECRETO
 *   POST { id, delete: true }                      -> revoca
 *
 * El secreto en claro solo viaja en la respuesta de creación (el control-plane no
 * lo vuelve a entregar). No se cachea ni se registra.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { createApiToken, revokeApiToken, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

const SCOPES = new Set(['read', 'write', 'deploy']);

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    name?: unknown;
    scope?: unknown;
    project_id?: unknown;
    expires_at?: unknown;
    id?: unknown;
    delete?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  try {
    if (body.delete === true) {
      const id = typeof body.id === 'string' ? body.id.trim() : '';
      if (!id) return json({ error: 'missing_id' }, 400);
      await revokeApiToken(session, id);
      return json({ ok: true, live: true });
    }

    const name = typeof body.name === 'string' ? body.name.trim() : '';
    const scope = typeof body.scope === 'string' ? body.scope.trim() : '';
    if (!name) return json({ error: 'missing_name' }, 400);
    if (!SCOPES.has(scope)) return json({ error: 'invalid_scope' }, 400);

    const { token, plaintext } = await createApiToken(session, {
      name,
      scope,
      ...(typeof body.project_id === 'string' && body.project_id.trim()
        ? { projectId: body.project_id.trim() }
        : {}),
      ...(typeof body.expires_at === 'string' && body.expires_at.trim()
        ? { expiresAt: body.expires_at.trim() }
        : {}),
    });
    // live siempre true: sin control-plane se devuelve 503, nunca un token inventado.
    return json({ ok: true, live: true, token, plaintext });
  } catch (err) {
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    // Sin control-plane no se finge un token: sería una credencial que no existe.
    return json({ error: 'control_plane_unavailable' }, 503);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
