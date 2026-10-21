/**
 * GET /api/emails?mailboxId=&limit=&cursor= — lista correos de una carpeta con
 * paginación KEYSET (cursor "received_at,id") contra el endpoint REST del
 * servicio Go (GET /api/v1/mailboxes/{id}/messages), cuyo coste por página es
 * constante aunque el buzón tenga cientos de miles de mensajes (sin OFFSET).
 * Proxy BFF autorizado por cookie. Devuelve { emails, nextCursor } — nextCursor
 * vacío = no hay más páginas.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, toEmailSummary } from '../../lib/mailrest.js';

export const prerender = false;

interface ListResponse {
  messages?: unknown[];
  next_cursor?: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const mailboxId = ctx.url.searchParams.get('mailboxId');
  if (!mailboxId || !/^[0-9a-f-]{36}$/i.test(mailboxId))
    return json({ error: 'missing_mailboxId' }, 400);
  const limit = clampInt(ctx.url.searchParams.get('limit'), 50, 1, 100);
  const cursor = ctx.url.searchParams.get('cursor') ?? '';

  const qs = new URLSearchParams({ limit: String(limit) });
  if (cursor) qs.set('cursor', cursor);

  try {
    const res = await mailGet(
      session.accessToken,
      `/api/v1/mailboxes/${encodeURIComponent(mailboxId)}/messages?${qs.toString()}`,
    );
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as ListResponse;
    const emails = (body.messages ?? []).map((m) => toEmailSummary(m as never));
    return json({ emails, nextCursor: body.next_cursor ?? '' });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

function clampInt(raw: string | null, fallback: number, min: number, max: number): number {
  const n = raw == null ? NaN : Number.parseInt(raw, 10);
  if (Number.isNaN(n)) return fallback;
  return Math.min(max, Math.max(min, n));
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
