/*
 * GET /api/db/rows?project=<slug>&schema=<schema>&table=<name>&limit=<n>
 *
 * BFF de Sytue DB Studio: devuelve las primeras filas de una tabla del Postgres
 * gestionado del proyecto. Consulta el control-plane (Sytue DevX API) con el
 * access token de la sesion via el proveedor server-only; degrada al seed de
 * demostracion si no hay backend. NUNCA expone tokens al cliente.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { loadDbRows } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const project = url.searchParams.get('project')?.trim();
  const schema = url.searchParams.get('schema')?.trim();
  const table = url.searchParams.get('table')?.trim();
  if (!project || !schema || !table) return json({ error: 'missing_params' }, 400);

  const rawLimit = Number(url.searchParams.get('limit'));
  const limit = Number.isFinite(rawLimit) && rawLimit > 0 ? Math.min(rawLimit, 500) : 50;

  const { live, data } = await loadDbRows(session, project, schema, table, limit);
  return json({ live, rows: data });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
