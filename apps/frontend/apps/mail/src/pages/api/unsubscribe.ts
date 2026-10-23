/**
 * /api/unsubscribe — darse de baja de listas (RFC 2369 List-Unsubscribe + RFC 8058
 * one-click). GET ?emailId= → { oneClick, mailto } (métodos disponibles, SIN exponer
 * URLs/direcciones al navegador). POST { emailId } → ejecuta la baja en el servicio
 * Go: one-click https SSRF-safe (POST List-Unsubscribe=One-Click) o envío del correo
 * de baja al mailto. Proxy BFF autorizado por cookie; el access token nunca llega al
 * navegador. POST same-origin: el checkOrigin CSRF de Astro cubre el disparo.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost } from '../../lib/mailrest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const emailId = ctx.url.searchParams.get('emailId');
  if (!emailId) return json({ error: 'missing_emailId' }, 400);

  try {
    const res = await mailGet(
      session.accessToken,
      `/api/v1/messages/${encodeURIComponent(emailId)}/unsubscribe`,
    );
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const data = (await res.json()) as { one_click?: boolean; mailto?: boolean };
    return json({ oneClick: Boolean(data.one_click), mailto: Boolean(data.mailto) });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { emailId?: string };
  if (!body.emailId) return json({ error: 'missing_emailId' }, 400);

  try {
    const res = await mailPost(
      session.accessToken,
      `/api/v1/messages/${encodeURIComponent(body.emailId)}/unsubscribe`,
      {},
    );
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const data = (await res.json()) as { status?: string };
    return json({ status: data.status ?? 'failed' });
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
