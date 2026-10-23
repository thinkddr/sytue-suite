/**
 * POST /api/seen — marca un correo como leido/no leido ($seen).
 * Cuerpo JSON: { emailId: string, seen: boolean }.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';

export const prerender = false;

interface Payload {
  emailId?: string;
  seen?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (!body.emailId || typeof body.seen !== 'boolean') {
    return json({ error: 'invalid_payload' }, 400);
  }
  try {
    await jmapClient(session.accessToken).setSeen(body.emailId, body.seen);
    return json({ ok: true });
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
