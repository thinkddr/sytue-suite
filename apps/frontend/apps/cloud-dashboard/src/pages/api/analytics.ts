/*
 * POST /api/analytics
 *   { host }                 -> empieza a medir ese hostname
 *   { site_id, delete:true } -> deja de medir (y borra los agregados)
 *   { site_id, events: [] }  -> declara que nombres de `data-sytue-event` se aceptan
 *
 * BFF de Sytue Analytics. El borrado viaja por POST porque el checkOrigin CSRF de
 * Astro rechaza DELETE con 403 (mismo patrón que /api/env y /api/uptime).
 *
 * El host NO se acepta a ciegas: solo se puede medir un hostname que sea DE ESTE
 * proyecto —el subdominio de la app o uno de sus dominios verificados—. Sin esa
 * comprobación, cualquiera con una cuenta podría reclamar `competencia.com` y,
 * como el host es único global en Analytics, dejar a su dueño sin poder medirlo.
 * NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import {
  enableAnalytics,
  disableAnalytics,
  updateEventAllowlist,
  analyticsBaseUrl,
  ownedHostsFrom,
} from '../../lib/analytics.js';
import { loadProject, loadDomains, httpErrorInfo } from '../../lib/cloud.js';
import type { SytueSession } from '@sytue/auth';

export const prerender = false;

/** Hostnames de este proyecto. El criterio vive en lib/analytics.ts y se prueba allí. */
async function ownedHosts(session: SytueSession, slug: string): Promise<Set<string> | null> {
  const [project, domains] = await Promise.all([
    loadProject(session, slug),
    loadDomains(session, slug),
  ]);
  return ownedHostsFrom(project, domains);
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  if (!analyticsBaseUrl()) return json({ error: 'analytics_not_configured' }, 503);

  let body: {
    project?: unknown;
    host?: unknown;
    site_id?: unknown;
    delete?: unknown;
    events?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  try {
    if (body.delete === true) {
      const siteId = typeof body.site_id === 'string' ? body.site_id.trim() : '';
      if (!siteId) return json({ error: 'missing_site_id' }, 400);
      await disableAnalytics(session, siteId);
      return json({ ok: true, live: true });
    }

    if (Array.isArray(body.events)) {
      const siteId = typeof body.site_id === 'string' ? body.site_id.trim() : '';
      if (!siteId) return json({ error: 'missing_site_id' }, 400);
      // El saneado de verdad (longitud, caracteres, tope) lo hace `cleanAllowlist`
      // en Go, que es quien decide: aqui solo se quita lo que no es texto para no
      // mandarle un array de objetos.
      const names = body.events.filter((e): e is string => typeof e === 'string');
      const site = await updateEventAllowlist(session, siteId, names);
      return json({ ok: true, live: true, site });
    }

    const project = typeof body.project === 'string' ? body.project.trim() : '';
    const host = typeof body.host === 'string' ? body.host.trim().toLowerCase() : '';
    if (!project || !host) return json({ error: 'missing_host' }, 400);

    const owned = await ownedHosts(session, project);
    if (owned === null) return json({ error: 'control_plane_unavailable' }, 503);
    if (!owned.has(host)) return json({ error: 'host_not_owned' }, 403);

    const site = await enableAnalytics(session, host);
    return json({ ok: true, live: true, site });
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
