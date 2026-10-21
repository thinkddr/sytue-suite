/*
 * POST /api/probe-health  { project: <slug> }
 *
 * Sondea la app del proyecto con las rutas de salud de convención y devuelve qué
 * contestó cada una. No cambia nada: la ruta que se elija se guarda después por
 * /api/project-runtime.
 *
 * El control-plane construye la URL a sondear a partir del proyecto; aquí NO se
 * acepta ninguna URL del cliente, que convertiría esto en un proxy de peticiones
 * arbitrarias. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { probeHealthPaths, httpErrorInfo } from '../../lib/cloud.js';

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
    const report = await probeHealthPaths(session, project);
    if (!report) return json({ error: 'unavailable' }, 503);
    return json({ ok: true, ...report });
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
