/*
 * POST /api/deploy
 *   { deployment }              -> vuelve a publicar esa produccion (rollback)
 *   { deployment, preview: true } -> publica en produccion la imagen de esa preview
 *   { deployment, cancel: true } -> cancela ese despliegue si sigue en curso
 *
 * BFF de despliegues. La accion viaja por POST (no PUT) porque el checkOrigin CSRF
 * de Astro rechaza otros verbos con 403 (mismo patron que /api/env). Devuelve
 * { ok, live }: live=false cuando no hay backend vivo (dev/local) y el cambio queda
 * en demostracion. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import {
  promoteDeployment,
  promotePreviewDeployment,
  cancelDeployment,
  triggerDeploy,
  httpErrorInfo,
} from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { deployment?: unknown; project?: unknown; cancel?: unknown; preview?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const deployment = typeof body.deployment === 'string' ? body.deployment.trim() : '';
  const project = typeof body.project === 'string' ? body.project.trim() : '';

  try {
    if (deployment) {
      // Cancelar y promover van por el mismo endpoint porque hablan del mismo objeto;
      // lo que cambia es el verbo contra el control-plane.
      // Tres verbos sobre el mismo objeto: cancelar, publicar una preview y
      // volver a una producción anterior. Cada uno es un endpoint distinto del
      // control-plane; el BFF no decide nada, solo enruta lo que pide el panel.
      let live: boolean;
      if (body.cancel === true) {
        live = await cancelDeployment(session, deployment);
      } else if (body.preview === true) {
        live = await promotePreviewDeployment(session, deployment);
      } else {
        live = await promoteDeployment(session, deployment);
      }
      return json({ ok: true, live });
    }
    if (project) {
      const live = await triggerDeploy(session, project);
      return json({ ok: true, live });
    }
  } catch (err) {
    // Error REAL del control-plane (quota_exceeded/conflict/forbidden/...): status y
    // código reales a la UI. Sin backend vivo, la mutación ya degradó a live:false.
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    throw err;
  }
  return json({ error: 'missing_deployment_or_project' }, 400);
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
