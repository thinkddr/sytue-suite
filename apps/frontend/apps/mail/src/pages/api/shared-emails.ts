/**
 * GET /api/shared-emails?accountId=&mailboxId=&cursor= — página (keyset) de una
 * carpeta de un buzón compartido/delegado. Proxy BFF al servicio Go, que EXIGE ACL
 * de lectura sobre la cuenta y verifica que la carpeta le pertenece (no se puede
 * leer una carpeta de otra cuenta pasando su id). Normaliza los sobres a
 * EmailSummary, como el resto de listados.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, toEmailSummary } from '../../lib/mailrest.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;

interface RawList {
  messages?: Parameters<typeof toEmailSummary>[0][];
  next_cursor?: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const accountId = ctx.url.searchParams.get('accountId') ?? '';
  const mailboxId = ctx.url.searchParams.get('mailboxId') ?? '';
  if (!UUID.test(accountId) || !UUID.test(mailboxId)) return json({ error: 'invalid_params' }, 400);

  const params = new URLSearchParams();
  const limit = ctx.url.searchParams.get('limit');
  const cursor = ctx.url.searchParams.get('cursor');
  if (limit) params.set('limit', limit);
  if (cursor) params.set('cursor', cursor);
  const qs = params.toString();

  try {
    const res = await mailGet(
      session.accessToken,
      `/api/v1/shared-mailboxes/${accountId}/mailboxes/${mailboxId}/messages${qs ? `?${qs}` : ''}`,
    );
    if (res.status === 403) return json({ error: 'forbidden' }, 403);
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as RawList;
    return json({
      emails: (body.messages ?? []).map(toEmailSummary),
      nextCursor: body.next_cursor ?? '',
    });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
