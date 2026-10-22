/**
 * POST /api/ws-ticket — entrega la credencial para que el navegador abra la
 * conexion de tiempo real contra el servicio Go Sytue Chat (/ws). El servicio Go
 * valida el access token directamente en el upgrade (?access_token=), igual que
 * el stream SSE del Hub; no hay minted-ticket separado. Devuelve {ticket, wsUrl}.
 * docs/CONVENTIONS.md §1.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const env = serverEnv(import.meta.env);
  const wsUrl = env.SYTUE_ROOMS_WS_URL ?? env.SYTUE_CHAT_WS_URL ?? 'wss://rooms.sytue.eu/ws';
  // El token (corta vida) es la credencial de WS; el navegador lo pasa por query
  // porque WebSocket no admite cabeceras (mismo patrón que el SSE del Hub).
  return json({ ticket: session.accessToken, wsUrl });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
