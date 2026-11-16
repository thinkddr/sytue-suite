/*
 * GET /api/deployments?project=<slug>  -> { live, deployment? }
 *
 * El despliegue MÁS RECIENTE del proyecto (estado y URL). Lo pregunta el asistente
 * de nuevo proyecto mientras enseña el log del primer build, para saber cuándo
 * dejar de mirar y decir «ya está en línea» o «ha fallado».
 *
 * Sin control-plane vivo NO se devuelve el despliegue de demostración: el
 * asistente diría que una app que no existe está publicada. Errores reales del
 * control-plane (403, 404…) llegan con su status. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { loadDeployments } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const project = new URL(ctx.request.url).searchParams.get('project')?.trim();
  if (!project) return json({ error: 'missing_project' }, 400);

  const { live, data, error } = await loadDeployments(session, project);
  if (error) return json({ error: error.code }, error.status);
  const latest = live ? data[0] : undefined;
  return json({
    live,
    ...(latest ? { deployment: { id: latest.id, status: latest.status, url: latest.url } } : {}),
  });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
