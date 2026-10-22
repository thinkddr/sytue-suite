/*
 * POST /api/projects/scaffold -> { ok, live, authorize_url? }
 *
 * La tercera vía del alta: no tengo repositorio, créamelo. Devuelve a dónde mandar
 * al usuario a autorizarlo en Sytue Git — el repositorio se crea EN SU CUENTA, así
 * que primero tiene que consentirlo, y Sytue DevX usa una aplicación OAuth2 de la
 * forja y no un token de administrador.
 *
 * `live: false` es la respuesta honesta sin control-plane: no hay nada que
 * autorizar porque no hay quien cree el repositorio. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { startScaffold, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as {
    template_id?: string;
    slug?: string;
    name?: string;
    repo_name?: string;
  };
  const templateId = (body.template_id ?? '').trim();
  const slug = (body.slug ?? '').trim().toLowerCase();
  if (!templateId || !slug) return json({ error: 'template_and_slug_required' }, 400);
  try {
    const url = await startScaffold(session, {
      template_id: templateId,
      slug,
      name: (body.name ?? '').trim() || slug,
      repo_name: (body.repo_name ?? '').trim(),
    });
    if (!url) return json({ ok: true, live: false });
    return json({ ok: true, live: true, authorize_url: url });
  } catch (err) {
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    // Sin status del control-plane no se ha creado nada y no hay a dónde mandar a
    // nadie: se responde como «no vivo» en vez de propagar un fallo opaco.
    return json({ ok: true, live: false });
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
