/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /api/drafts — BORRADORES del usuario (proxy BFF hacia el servicio Go de Sytue
 * Rooms, autorizado por cookie; el access token no toca el navegador).
 *
 *   GET                                     → mis borradores (cross-canal)
 *   POST { channelId, parentId?, body }      → guarda; body en BLANCO lo BORRA
 *
 * La escritura va por POST (y no por PUT/DELETE) porque el checkOrigin de Astro
 * rechaza esos métodos tras el proxy; es el mismo patrón que /api/pin y /api/save.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const drafts = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listDrafts();
    return json({ drafts });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { channelId?: string; parentId?: string; body?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const channelId = payload.channelId?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);

  try {
    await chatClient({ accessToken: session.accessToken, userId: session.user.id }).putDraft(
      channelId,
      payload.parentId?.trim() || undefined,
      payload.body ?? '',
    );
    return json({ ok: true });
  } catch (err) {
    return json({ error: 'chat_error' }, 502);
  }
};
