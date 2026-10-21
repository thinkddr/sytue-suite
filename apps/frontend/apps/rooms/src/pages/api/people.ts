/**
 * POST /api/people — encuentra a alguien de CUALQUIER organización de Sytue One por su
 * @usuario o su correo, para abrirle un mensaje, añadirle a un canal o llamarle
 * (ADR 0040). Cuerpo: { query }. Devuelve { person: { sub, name, username? } }.
 *
 * Proxy BFF puro hacia el servicio Go, que pregunta a Sytue ID con el token de la
 * sesión: el límite por persona lo aplica Sytue ID, y aquí se PRESERVAN sus 4xx para
 * que el buscador distinga «no existe» (404) de «espera un momento» (429).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, ChatHttpError } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { query?: unknown };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const query = typeof payload.query === 'string' ? payload.query.trim() : '';
  if (!query) return json({ error: 'missing_query' }, 400);

  try {
    const person = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).resolvePerson(query);
    return json({ person });
  } catch (err) {
    if (err instanceof ChatHttpError && err.status >= 400 && err.status < 500) {
      return json({ error: 'chat_error' }, err.status);
    }
    return json({ error: 'chat_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
