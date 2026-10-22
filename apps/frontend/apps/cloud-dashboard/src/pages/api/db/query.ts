/*
 * POST /api/db/query  { project: <slug>, sql: <string> }
 *
 * BFF de Sytue DB Studio: ejecuta SQL contra el Postgres del proyecto (rol
 * acotado, solo lectura por defecto) via el control-plane (Sytue DevX API) con
 * el access token de la sesion. Si no hay backend, evalua SELECT en el seed como
 * demostracion. NUNCA expone tokens ni cadenas de conexion al cliente.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { runSql } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { project?: unknown; sql?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const project = typeof body.project === 'string' ? body.project.trim() : '';
  const sql = typeof body.sql === 'string' ? body.sql : '';
  if (!project) return json({ error: 'missing_project' }, 400);
  if (!sql.trim()) return json({ error: 'empty_sql' }, 400);

  const result = await runSql(session, project, sql, ctx.locals.locale);
  return json(result);
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
