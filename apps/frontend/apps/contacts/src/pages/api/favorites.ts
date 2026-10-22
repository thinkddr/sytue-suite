/**
 * Favoritos (destacados) — proxy BFF autorizado por cookie hacia la API REST del
 * servicio Go Sytue Contacts.
 *
 * GET  /api/favorites               -> GET  /api/v1/favorites            { uids }
 * POST /api/favorites { uid, favorite } -> POST /api/v1/favorites (toggle)
 *
 * La mutación va por POST con cuerpo JSON (CSRF-safe: el navegador no puede
 * enviarla cross-origin sin preflight; Astro además rechaza DELETE/PUT). El
 * servicio Go destaca por UID y SOLO contactos del propio dueño. El access token
 * nunca llega al navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { restJson, RestError } from '../../lib/rest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const body = await restJson<{ uids: string[] }>(
      session.accessToken,
      'GET',
      '/api/v1/favorites',
    );
    return json(body);
  } catch (err) {
    return fail(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { uid?: string; favorite?: boolean };
  if (!body.uid?.trim()) return json({ error: 'missing_uid' }, 400);

  try {
    await restJson<unknown>(session.accessToken, 'POST', '/api/v1/favorites', {
      uid: body.uid.trim(),
      favorite: body.favorite === true,
    });
    return json({ ok: true });
  } catch (err) {
    return fail(err);
  }
};

function fail(err: unknown): Response {
  if (err instanceof RestError)
    return json({ error: 'contacts_error', message: err.message }, err.status);
  return json({ error: 'contacts_error', message: String(err) }, 502);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
