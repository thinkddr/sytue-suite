/*
 * POST /api/deploy-freeze { project, frozen: true, reason? } -> congela producción
 * POST /api/deploy-freeze { project, frozen: false }         -> la descongela
 *
 * BFF de la congelación de despliegues. Proxy PURO al control-plane
 * (PUT/DELETE /api/v1/projects/{id}/deploy-freeze); sin lógica (invariante). El
 * descongelar viaja por POST porque el checkOrigin CSRF de Astro rechaza DELETE
 * con 403 (mismo patrón que /api/preview-protection). La autorización
 * (admin/owner, solo humanos) la impone el control-plane: aquí solo se propaga su
 * 403 para que la pantalla diga que hace falta otro rol. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { setDeployFreeze, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { project?: unknown; frozen?: unknown; reason?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const project = typeof body.project === 'string' ? body.project.trim() : '';
  if (!project) return json({ error: 'missing_project' }, 400);
  if (typeof body.frozen !== 'boolean') return json({ error: 'missing_frozen' }, 400);
  const reason = typeof body.reason === 'string' ? body.reason : '';

  try {
    const estado = await setDeployFreeze(
      session,
      project,
      body.frozen ? { frozen: true, reason } : { frozen: false },
    );
    // Sin backend vivo, el cambio queda en demostración: se devuelve lo pedido.
    return json({ ok: true, live: Boolean(estado), ...(estado ?? { frozen: body.frozen }) });
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
