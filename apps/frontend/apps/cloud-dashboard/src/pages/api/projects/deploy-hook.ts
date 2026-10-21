/*
 * POST /api/projects/deploy-hook { project } -> emite la URL de despliegue.
 *
 * Por debajo crea un token de API con scope `hook` CONFINADO a ese proyecto
 * (solo vale en la ruta del disparador, no en la API: CLOUD-17) y
 * devuelve su secreto, que es lo que el panel pega en la ruta pública
 * /hooks/deploy/{token}. Se revoca donde se revocan los demás, en Tokens.
 *
 * Devuelve el secreto UNA vez y no se registra en ningún sitio: quien tenga esa
 * URL puede publicar el proyecto.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { createDeployHookToken, httpErrorInfo } from '../../../lib/cloud.js';

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
    const token = await createDeployHookToken(session, project);
    if (!token) return json({ error: 'unavailable' }, 503);
    return json({ ok: true, live: true, token });
  } catch (err) {
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    return json({ error: 'control_plane_unavailable' }, 503);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
