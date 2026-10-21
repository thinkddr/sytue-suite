/*
 * POST /api/suggest-slug -> { slug, available }
 *
 * Nombre libre sugerido para un proyecto nuevo. Va por POST y no por GET porque
 * cada llamada devuelve un nombre distinto: como GET sería cacheable y acabaría
 * proponiendo siempre el mismo (y ya ocupado) desde cualquier caché por el camino.
 *
 * El control-plane genera Y comprueba contra el índice global de slugs; aquí solo
 * se hace de tubería. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { suggestProjectSlug, httpErrorInfo } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as { lang?: string };
  try {
    // El nombre se genera con la gramática del idioma negociado (§13.1): idioma
    // explícito de la isla si viene y, si no, la cabecera del navegador tal cual.
    const acceptLanguage = ctx.request.headers.get('accept-language');
    const suggestion = await suggestProjectSlug(session, {
      ...(body.lang ? { lang: body.lang } : {}),
      ...(acceptLanguage ? { acceptLanguage } : {}),
    });
    if (!suggestion) return json({ error: 'unavailable' }, 503);
    return json({ ok: true, ...suggestion });
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
