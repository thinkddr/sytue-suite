/**
 * POST /api/send — publica un mensaje en un canal (proxy BFF hacia el servicio Go
 * Sytue Chat). Cuerpo: { channelId, body, clientId?, mentions?, fileIds? }. Un
 * mensaje puede llevar solo adjuntos (cuerpo vacío). El servidor reenvia el
 * mensaje y el resto de clientes lo reciben por WebSocket.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient, type MessageCard } from '../../lib/chat.js';
import { reference } from '../../client/matrix/types.js';
import { sanitizeFileIds } from '../../lib/fileIds.js';

/** Deja pasar solo la DEFINICIÓN de una tarjeta conocida (encuesta/tarea); el estado
 *  (`votes`) y cualquier campo de más se descartan aquí, antes de tocar el servicio.
 *  El servicio Go vuelve a validarla: esta es una verja de forma, no la autoridad. */
function sanitizeCard(raw: unknown): MessageCard | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const c = raw as Record<string, unknown>;
  if (c.kind !== 'poll' && c.kind !== 'task') return undefined;
  const options = Array.isArray(c.options)
    ? c.options.filter((o): o is string => typeof o === 'string' && o.trim() !== '')
    : [];
  return {
    kind: c.kind,
    ...(options.length ? { options } : {}),
    ...(c.multi === true ? { multi: true } : {}),
    ...(typeof c.assigneeSub === 'string' && c.assigneeSub ? { assigneeSub: c.assigneeSub } : {}),
    ...(typeof c.assigneeName === 'string' && c.assigneeName
      ? { assigneeName: c.assigneeName }
      : {}),
    ...(typeof c.due === 'string' && c.due ? { due: c.due } : {}),
  };
}

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    matrix_event?: unknown;
    channelId?: string;
    body?: string;
    clientId?: string;
    mentions?: string[];
    fileIds?: unknown;
    type?: string;
    priority?: string;
    requiresAck?: boolean;
    card?: unknown;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const matrixEvent = reference(payload.matrix_event);
  const channelId = payload.channelId?.trim();
  const body = payload.body?.trim() ?? '';
  const fileIds = sanitizeFileIds(payload.fileIds);
  // 'call' es una tarjeta de sistema (Sytue Rooms): cuerpo vacío permitido.
  const type = payload.type === 'call' ? 'call' : undefined;
  // Prioridad (Ola 12): solo se reenvía si es un valor válido no-normal.
  const priority =
    payload.priority === 'important' || payload.priority === 'urgent'
      ? payload.priority
      : undefined;
  const card = sanitizeCard(payload.card);
  if (!channelId) return json({ error: 'missing_channelId' }, 400);
  // Una encuesta o una tarea SIN texto no la puede leer nadie: el cuerpo es la
  // pregunta o el título, y ahí no vale el atajo de «cuerpo vacío con adjuntos».
  if (matrixEvent && (body || fileIds.length || card || payload.mentions?.length))
    return json({ error: 'plaintext_not_allowed' }, 400);
  if (card && !body) return json({ error: 'empty_body' }, 400);
  // Cuerpo vacío permitido si el mensaje lleva adjuntos o es una tarjeta de llamada.
  if (!body && fileIds.length === 0 && !type && !matrixEvent)
    return json({ error: 'empty_body' }, 400);
  if (body.length > 8000) return json({ error: 'body_too_long' }, 413);
  const mentions = Array.isArray(payload.mentions)
    ? payload.mentions.filter((m): m is string => typeof m === 'string' && m.trim() !== '')
    : undefined;

  try {
    const message = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).sendMessage({
      channelId,
      body,
      ...(matrixEvent ? { matrix_event: matrixEvent } : {}),
      ...(type ? { type } : {}),
      ...(payload.clientId ? { clientId: payload.clientId } : {}),
      ...(mentions?.length ? { mentions } : {}),
      ...(fileIds.length ? { fileIds } : {}),
      ...(priority ? { priority } : {}),
      ...(payload.requiresAck === true ? { requiresAck: true } : {}),
      ...(card ? { card } : {}),
    });
    return json({ message });
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
