/**
 * /api/undo-send — ventana de deshacer el envío del usuario (5/10/20/30 s), proxy
 * BFF al servicio Go (REST /api/v1/undo-send). Era una constante del cliente; ahora
 * es un valor por CUENTA en el servidor, el mismo en cualquier equipo.
 *
 * GET lee; POST guarda (el checkOrigin CSRF de Astro rechaza el PUT del navegador,
 * así que el BFF traduce POST → PUT, igual que /api/signature). Qué valores valen
 * lo decide el servicio: su 400 se propaga para que el cliente no dé por guardado
 * algo que no se guardó.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailOrigin } from '../../lib/mailrest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/undo-send');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { seconds?: number };
    return json({ seconds: body.seconds ?? 0 });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as { seconds?: unknown };
  try {
    const res = await fetch(`${mailOrigin()}/api/v1/undo-send`, {
      method: 'PUT',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify({ seconds: typeof body.seconds === 'number' ? body.seconds : 0 }),
    });
    if (!res.ok)
      return json({ error: 'mail_error', status: res.status }, res.status === 400 ? 400 : 502);
    return json({ ok: true });
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
