/**
 * POST /api/dm — abre (idempotente) un DM (1:1 o de grupo) con otros miembros del team
 * (proxy BFF hacia el servicio Go Sytue Chat). Cuerpo: { subjects: [...] } (canónico) o
 * { subject } (1:1, retrocompatible). Devuelve { channel } (nuevo o reutilizado).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { chatClient } from '../../lib/chat.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { subject?: string; subjects?: unknown };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  // Acepta la lista (grupo o 1:1) o el subject único (1:1); normaliza a subs no vacíos.
  const subjects = Array.isArray(payload.subjects)
    ? payload.subjects.map((s) => String(s).trim()).filter(Boolean)
    : payload.subject?.trim()
      ? [payload.subject.trim()]
      : [];
  if (subjects.length === 0) return json({ error: 'missing_subject' }, 400);

  try {
    const channel = await chatClient({
      accessToken: session.accessToken,
      userId: session.user.id,
    }).openDirect(subjects);
    return json({ channel });
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
