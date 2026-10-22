/*
 * POST /api/check-slug -> { ok, live, slug, available, alternatives }
 *
 * ¿Está libre el nombre que estoy escribiendo? El slug es el subdominio público y
 * su índice es único en TODA la plataforma, así que sin esto el usuario no se entera
 * hasta el 409 de la creación —y ese 409 no puede decirle que el nombre lo tiene
 * otra organización—. Cuando está cogido, el control-plane devuelve alternativas
 * libres derivadas de SU nombre.
 *
 * Va por POST y no por GET porque la respuesta caduca en cuanto alguien crea un
 * proyecto: como GET sería cacheable y un «libre» viejo lleva justo al 409 que esto
 * viene a evitar.
 *
 * `live: false` es la respuesta honesta cuando el control-plane no está conectado:
 * sin él NO se puede saber si un nombre está libre, y la interfaz lo dice en vez de
 * inventarse un «disponible». NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { checkProjectSlug, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as { slug?: string; lang?: string };
  const slug = (body.slug ?? '').trim().toLowerCase();
  if (!slug) return json({ error: 'slug_required' }, 400);
  try {
    // El idioma lo decide el control-plane: parámetro explícito de la isla y, si no
    // viene, la cabecera del navegador tal cual (la negociación —q ausente = 1,
    // ordenar por q, saltar los idiomas que no hablamos— vive testeada en Go).
    const result = await checkProjectSlug(session, slug, {
      ...(body.lang ? { lang: body.lang } : {}),
      ...headerLang(ctx.request),
    });
    if (!result) return json({ ok: true, live: false });
    return json({ ok: true, live: true, ...result });
  } catch (err) {
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    throw err;
  }
};

function headerLang(request: Request): { acceptLanguage?: string } {
  const value = request.headers.get('accept-language');
  return value ? { acceptLanguage: value } : {};
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
