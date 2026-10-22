/*
 * POST /api/restart  { project: <slug> }
 *
 * Reinicio rodante de la producción del proyecto: no compila nada y no cambia lo
 * publicado. Devuelve { ok, live, restarted }; live=false cuando no hay backend
 * vivo (dev/local) y la acción se queda en demostración. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { restartApp, httpErrorInfo } from '../../lib/cloud.js';

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
    const restarted = await restartApp(session, project);
    return json({ ok: true, live: restarted !== undefined, restarted: restarted ?? 0 });
  } catch (err) {
    // 409 con el proyecto pausado, 404 sin producción activa: el código real sube
    // a la interfaz, que es la que sabe explicar cada caso.
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
