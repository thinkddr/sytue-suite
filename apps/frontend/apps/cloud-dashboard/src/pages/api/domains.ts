/*
 * POST /api/domains
 *   { project, hostname }           -> añade dominio
 *   { project, domain_id, delete: true } -> elimina dominio
 *
 * Devuelve { ok, live }. live=false en modo demostración.
 *
 * El alta devuelve además `domain` con los DOS registros DNS que el
 * control-plane calcula: el CNAME (a dónde mandar el tráfico) y el TXT de
 * verificación (la prueba de que la zona es del cliente). Se descartaban, así que
 * la pantalla prometía un token que jamás entregaba y ningún dominio propio podía
 * llegar a verificarse.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { addDomain, removeDomain, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

const HOST_RE = /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    project?: unknown;
    hostname?: unknown;
    domain_id?: unknown;
    delete?: unknown;
    set_primary?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const project = typeof body.project === 'string' ? body.project.trim() : '';
  if (!project) return json({ error: 'missing_project' }, 400);

  try {
    if (body.delete === true) {
      const domainId = typeof body.domain_id === 'string' ? body.domain_id.trim() : '';
      if (!domainId) return json({ error: 'missing_domain_id' }, 400);
      const live = await removeDomain(session, domainId);
      return json({ ok: true, live });
    }

    if (body.set_primary === true) {
      const domainId = typeof body.domain_id === 'string' ? body.domain_id.trim() : '';
      if (!domainId) return json({ error: 'missing_domain_id' }, 400);
      const { setDomainPrimary } = await import('../../lib/cloud.js');
      const live = await setDomainPrimary(session, domainId);
      return json({ ok: true, live });
    }

    const hostname = typeof body.hostname === 'string' ? body.hostname.trim().toLowerCase() : '';
    if (!HOST_RE.test(hostname)) return json({ error: 'invalid_hostname' }, 400);

    const added = await addDomain(session, project, hostname);
    return json({ ok: true, live: added.live, domain: added.domain });
  } catch (err) {
    // Error REAL del control-plane (conflict/forbidden/invalid_state/...): status real a la UI.
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
