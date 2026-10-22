/**
 * POST /api/reply — publica una respuesta en el hilo de un mensaje raíz (proxy
 * BFF hacia el servicio Go Sytue Chat). Cuerpo: { messageId, body, mentions?,
 * fileIds? }. Una respuesta puede llevar solo adjuntos (cuerpo vacío). El resto
 * de clientes recibe la respuesta por WebSocket.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';
import { reference } from '../../client/matrix/types.js';
import { sanitizeFileIds } from '../../lib/fileIds.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    matrix_event?: unknown;
    messageId?: string;
    body?: string;
    mentions?: string[];
    fileIds?: unknown;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const matrixEvent = reference(payload.matrix_event);
  const messageId = payload.messageId?.trim();
  const body = payload.body?.trim() ?? '';
  const fileIds = sanitizeFileIds(payload.fileIds);
  if (!messageId) return json({ error: 'missing_messageId' }, 400);
  if (matrixEvent && (body || fileIds.length || payload.mentions?.length))
    return json({ error: 'plaintext_not_allowed' }, 400);
  if (!body && fileIds.length === 0 && !matrixEvent) return json({ error: 'empty_body' }, 400);
  if (body.length > 8000) return json({ error: 'body_too_long' }, 413);
  const mentions = Array.isArray(payload.mentions)
    ? payload.mentions.filter((m): m is string => typeof m === 'string' && m.trim() !== '')
    : undefined;

  try {
    const message = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).postReply(messageId, {
      body,
      ...(matrixEvent ? { matrix_event: matrixEvent } : {}),
      ...(mentions?.length ? { mentions } : {}),
      ...(fileIds.length ? { fileIds } : {}),
    });
    return json({ message });
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
