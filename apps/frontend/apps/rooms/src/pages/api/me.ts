/**
 * GET /api/me — lo que la interfaz necesita saber del PROPIO usuario para decidir qué
 * le ofrece (proxy BFF hacia el servicio Go Sytue Rooms).
 *
 * Hoy solo `workspaceAdmin`, con la misma definición que aplica el backend en
 * isWorkspaceAdmin: ser admin de algún team del tenant. Sirve para no ofrecer acciones
 * que el servidor va a denegar — no para autorizar: quien llame igualmente a la API de
 * integraciones seguirá recibiendo 403. La comprobación de verdad está en Go; esto es
 * solo cortesía de interfaz.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  try {
    const workspaceAdmin = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).isWorkspaceAdmin();
    return json({ workspaceAdmin });
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
