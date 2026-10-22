/**
 * /api/channels — canales del usuario (proxy BFF autorizado por cookie hacia el
 * servicio Go Sytue Chat). El access token no toca el navegador.
 *   GET                            → { channels: [...] }
 *   POST { name, kind, topic? }    → crea el canal; devuelve { channel } con 201
 * Sin lógica de negocio: el nombre lo valida y lo convierte a slug el servicio Go, que
 * también decide si el usuario puede crear en su team.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, ChatHttpError } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  try {
    const channels = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listChannels();
    return json({ channels });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { name?: string; kind?: string; topic?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const name = payload.name?.trim();
  if (!name) return json({ error: 'missing_name' }, 400);
  // Verja del tipo: el servicio también acepta 'direct' en ValidChannelKind, y un DM
  // se abre por /api/dm. Aquí solo se crean canales público/privado.
  const kind = payload.kind === 'private' ? 'private' : 'public';
  const topic = payload.topic?.trim();

  try {
    const channel = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).createChannel({ name, kind, ...(topic ? { topic } : {}) });
    return json({ channel }, 201);
  } catch (err) {
    // Los 4xx del servicio se PRESERVAN (409 nombre duplicado, 400 nombre inválido,
    // 403 sin permiso): el modal los pinta junto al campo. El resto colapsa a 502.
    if (err instanceof ChatHttpError && err.status >= 400 && err.status < 500) {
      return json({ error: 'chat_error' }, err.status);
    }
    return json({ error: 'chat_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
