/*
 * POST /api/webhook-secret { project } -> emite un secreto de webhook nuevo.
 *
 * BFF de rotación. Proxy PURO al control-plane
 * (POST /api/v1/projects/{id}/webhook-secret:rotate); sin lógica (invariante).
 *
 * Devuelve el secreto EN CLARO en el cuerpo, con `no-store`: es su única razón
 * de existir —hay que pegarlo en el proveedor git— y no se puede volver a leer
 * por esta vía. El anterior deja de valer en el acto, así que la interfaz avisa
 * antes de disparar. La autorización (admin/owner, solo humanos) la impone el
 * control-plane. NUNCA expone tokens de sesión.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { rotateWebhookSecret, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { project?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const project = typeof body.project === 'string' ? body.project.trim() : '';
  if (!project) return json({ error: 'missing_project' }, 400);

  try {
    const res = await rotateWebhookSecret(session, project);
    // Sin backend vivo no hay credencial que emitir: live=false y ningún valor
    // inventado, que alguien pegaría en su proveedor git para nada.
    return json({
      ok: true,
      live: Boolean(res),
      ...(res ? { secret: res.secret, url: res.url } : {}),
    });
  } catch (err) {
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    throw err;
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
