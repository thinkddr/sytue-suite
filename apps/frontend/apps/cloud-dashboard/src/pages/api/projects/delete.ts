/*
 * POST /api/projects/delete { project } -> da de baja el proyecto entero.
 *
 * BFF de baja de proyecto. Proxy PURO al control-plane (DELETE
 * /api/v1/projects/{id}); sin lógica (invariante). Va por POST porque el
 * checkOrigin CSRF de Astro rechaza DELETE con 403 (mismo patrón que /api/env).
 *
 * Es lo más destructivo del panel —se lleva despliegues, servicios gestionados y
 * los tokens acotados al proyecto—, y aun así no existía: un proyecto creado por
 * error se quedaba para siempre en la lista. La confirmación (escribir el nombre)
 * la impone la interfaz; la autorización (admin/owner) el control-plane. Devuelve
 * { ok, live }. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { deleteProject, httpErrorInfo } from '../../../lib/cloud.js';

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
    const live = await deleteProject(session, project);
    return json({ ok: true, live });
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
