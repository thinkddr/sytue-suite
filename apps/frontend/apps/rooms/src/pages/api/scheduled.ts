/**
 * /api/scheduled — mensajes programados del usuario (Ola 12; proxy BFF hacia el
 * servicio Go Sytue Chat, autorizado por cookie; el access token no toca el
 * navegador).
 *
 *   GET  ?channelId=  → lista mis programados (opcionalmente por canal).
 *   POST → crea / actualiza / cancela según el cuerpo (JSON, no form, contra el CSRF
 *          de Astro dev; la cancelación va por POST {cancel:true}, como /api/pin):
 *            { channelId, body, sendAt, fileIds?, mentions? } → programar
 *            { id, body?, sendAt?, fileIds? }                 → editar
 *            { id, cancel: true }                             → cancelar
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';
import { sanitizeFileIds } from '../../lib/fileIds.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const channelId = ctx.url.searchParams.get('channelId')?.trim();
  try {
    const scheduled = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).listScheduled(channelId || undefined);
    return json({ scheduled });
  } catch {
    return json({ error: 'chat_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    id?: string;
    channelId?: string;
    body?: string;
    sendAt?: string;
    fileIds?: unknown;
    mentions?: string[];
    cancel?: boolean;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const client = chatClient({ accessToken: session.accessToken, userId: session.user.id });
  try {
    const id = payload.id?.trim();
    if (id && payload.cancel === true) {
      await client.cancelScheduled(id);
      return json({ ok: true });
    }
    if (id) {
      const scheduled = await client.updateScheduled(id, {
        ...(payload.body !== undefined ? { body: payload.body.trim() } : {}),
        ...(payload.sendAt !== undefined ? { sendAt: payload.sendAt } : {}),
        ...(payload.fileIds !== undefined ? { fileIds: sanitizeFileIds(payload.fileIds) } : {}),
      });
      return json({ scheduled });
    }
    // Crear un programado.
    const channelId = payload.channelId?.trim();
    const body = payload.body?.trim() ?? '';
    const sendAt = payload.sendAt?.trim();
    const fileIds = sanitizeFileIds(payload.fileIds);
    if (!channelId) return json({ error: 'missing_channelId' }, 400);
    if (!sendAt) return json({ error: 'missing_sendAt' }, 400);
    if (!body && fileIds.length === 0) return json({ error: 'empty_body' }, 400);
    if (body.length > 8000) return json({ error: 'body_too_long' }, 413);
    const mentions = Array.isArray(payload.mentions)
      ? payload.mentions.filter((m): m is string => typeof m === 'string' && m.trim() !== '')
      : undefined;
    const scheduled = await client.scheduleMessage({
      channelId,
      body,
      sendAt,
      ...(mentions?.length ? { mentions } : {}),
      ...(fileIds.length ? { fileIds } : {}),
    });
    return json({ scheduled }, 201);
  } catch {
    return json({ error: 'chat_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
