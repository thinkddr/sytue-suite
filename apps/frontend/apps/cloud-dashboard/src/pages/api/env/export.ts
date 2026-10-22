/*
 * GET /api/env/export?project=<slug>[&target=production|preview]
 *
 * BFF de descarga del .env. Reenvía el fichero del control-plane TAL CUAL (tipo,
 * nombre y cuerpo), sin materializarlo ni registrarlo: lo que viaja son los
 * secretos del proyecto en claro. La autorización (admin/owner, solo humanos) y
 * la auditoría las impone el control-plane.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { streamEnvExport } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const project = url.searchParams.get('project')?.trim();
  if (!project) return json({ error: 'missing_project' }, 400);
  const target = url.searchParams.get('target') === 'preview' ? 'preview' : 'production';

  const upstream = await streamEnvExport(session, project, target);
  if (!upstream) return json({ error: 'unavailable' }, 503);
  return upstream;
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
