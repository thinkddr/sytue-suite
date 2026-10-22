/*
 * GET /api/db/credentials?project=<slug> -> cadena de conexión con contraseña.
 *
 * BFF de credenciales de la base del proyecto. Proxy PURO al control-plane
 * (GET /api/v1/projects/{ref}/db/credentials); sin lógica (invariante). La
 * autorización real (admin/owner, solo humanos) y la auditoría las impone el
 * control-plane.
 *
 * `no-store` y nada de logs: lo que devuelve es la llave de la base entera.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { dbConnectionString, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const project = new URL(ctx.request.url).searchParams.get('project')?.trim();
  if (!project) return json({ error: 'missing_project' }, 400);

  try {
    const dsn = await dbConnectionString(session, project);
    if (!dsn) return json({ error: 'unavailable' }, 503);
    return json({ connection_string: dsn });
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
