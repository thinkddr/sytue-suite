/**
 * GET /api/members — miembros del team del usuario, para el selector de menciones
 * del composer (proxy BFF hacia el servicio Go Sytue Chat). Rellena el nombre del
 * propio usuario desde la sesión: el backend puede tenerlo vacío para el sub de la
 * sesión (se persiste al publicar, no siempre al unirse), y sin nombre la mención
 * caería a un UUID.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  try {
    const members = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listMembers();
    const selfName = session.user.name?.trim();
    const withSelf = members.map((m) =>
      m.sub === session.user.id && selfName ? { ...m, name: selfName } : m,
    );
    return json({ members: withSelf });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
