/**
 * GET /api/search?q= — búsqueda global de Rooms, cross-team (proxy BFF hacia el
 * servicio Go Sytue Chat). La query puede incluir operadores from:/in:/before:/
 * after:, que se reenvían tal cual. El access token no toca el navegador. Los
 * resultados NO llevan el nombre del canal: el cliente lo resuelve contra su lista
 * de canales. docs/CONVENTIONS.md §8.
 */
import type { APIRoute } from 'astro';
import { consulta } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

const buscar: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = await consulta(ctx.request, ctx.url);
  const q = (params.get('q') ?? '').trim();
  if (!q) return json({ messages: [] });

  try {
    const messages = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).searchAll(q, 30);
    return json({ messages });
  } catch (err) {
    return json({ error: 'chat_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

// GET y QUERY, el mismo handler. QUERY es el preferente del cliente y GET el
// respaldo para quien no lo hable; ver `consulta` en @sytue/auth/server.
export const GET: APIRoute = buscar;
export const QUERY: APIRoute = buscar;
