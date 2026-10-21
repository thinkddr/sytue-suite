/*
 * POST /api/projects/clone { project, slug } -> crea un proyecto igual que otro.
 *
 * BFF de clonado. Proxy PURO al control-plane (POST /api/v1/projects/{id}/clone);
 * sin lógica (invariante). Devuelve { ok, live, slug, copied }: live=false cuando
 * no hay backend vivo y no se ha creado nada.
 *
 * El secreto de webhook del proyecto NUEVO viaja en la respuesta del
 * control-plane y NO se reenvía al navegador: quien lo necesite lo rota desde los
 * ajustes del proyecto clonado, que es el camino que ya existe para verlo una vez.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { cloneProject, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { project?: unknown; slug?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const project = typeof body.project === 'string' ? body.project.trim() : '';
  const slug = typeof body.slug === 'string' ? body.slug.trim() : '';
  if (!project || !slug) return json({ error: 'missing_params' }, 400);

  try {
    const copied = await cloneProject(session, project, slug);
    return json({ ok: true, live: copied !== undefined, slug, copied: copied ?? 0 });
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
