/**
 * GET /api/shared-folders?accountId= — carpetas de un buzón compartido/delegado.
 * Proxy BFF al servicio Go (REST /api/v1/shared-mailboxes/{accountId}/mailboxes),
 * que EXIGE ACL de lectura sobre esa cuenta (403 si no). Normaliza el
 * domain.Mailbox (snake_case) a la forma camelCase que consume el cliente.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet } from '../../lib/mailrest.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;

interface RawMailbox {
  id: string;
  name?: string;
  role?: string;
  total_emails?: number;
  unread_emails?: number;
}

function mapMailbox(m: RawMailbox) {
  return {
    id: m.id,
    name: m.name ?? '',
    role: m.role ?? '',
    totalEmails: m.total_emails ?? 0,
    unreadEmails: m.unread_emails ?? 0,
  };
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const accountId = ctx.url.searchParams.get('accountId') ?? '';
  if (!UUID.test(accountId)) return json({ error: 'invalid_account' }, 400);
  try {
    const res = await mailGet(
      session.accessToken,
      `/api/v1/shared-mailboxes/${accountId}/mailboxes`,
    );
    if (res.status === 403) return json({ error: 'forbidden' }, 403);
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { mailboxes?: RawMailbox[] };
    return json({ mailboxes: (body.mailboxes ?? []).map(mapMailbox) });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
