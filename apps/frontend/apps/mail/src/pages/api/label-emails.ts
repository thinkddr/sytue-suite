/**
 * GET /api/label-emails?labelId=&limit=&cursor= — mensajes de una etiqueta con
 * paginación KEYSET (cursor "received_at,id") contra el servicio Go
 * (GET /api/v1/labels/{id}/messages). Coste por página acotado como el listado de
 * carpeta; la etiqueta cruza carpetas (vista estilo Gmail). Proxy BFF por cookie;
 * devuelve { emails, nextCursor } — nextCursor vacío = no hay más.
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

  const labelId = ctx.url.searchParams.get('labelId');
  if (!labelId || !/^[0-9a-f-]{36}$/i.test(labelId)) return json({ error: 'missing_labelId' }, 400);
  const limit = clampInt(ctx.url.searchParams.get('limit'), 50, 1, 100);
  const cursor = ctx.url.searchParams.get('cursor') ?? '';

  const qs = new URLSearchParams({ limit: String(limit) });
  if (cursor) qs.set('cursor', cursor);

  try {
    const res = await mailGet(
      session.accessToken,
      `/api/v1/labels/${encodeURIComponent(labelId)}/messages?${qs.toString()}`,
    );
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as ListResponse;
    const emails = (body.messages ?? []).map((m) => toEmailSummary(m as never));
    return json({ emails, nextCursor: body.next_cursor ?? '' });
  } catch {
    return json({ error: 'mail_error' }, 502);
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
