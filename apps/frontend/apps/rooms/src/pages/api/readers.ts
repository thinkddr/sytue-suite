/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/readers?messageId=… — quién ha visto un mensaje y cuándo lo vio cada
 * uno. Proxy BFF hacia el servicio Go; la autorización (pertenecer al canal) la
 * impone el servicio, que es donde vive la verja.
 *
 * Es GET y no POST porque no cambia nada, y el identificador de un mensaje no es
 * dato personal: lo que sale del cuerpo sí, y por eso nunca va en la URL.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const messageId = new URL(ctx.request.url).searchParams.get('messageId')?.trim();
  if (!messageId) return json({ error: 'missing_messageId' }, 400);

  try {
    const data = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).messageReaders(messageId);
    return json(data);
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
