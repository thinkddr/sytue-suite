/**
 * /api/signature — firma de correo del usuario, proxy BFF al servicio Go (REST
 * /api/v1/signature). Vivía en localStorage: no existía al cambiar de equipo ni
 * al abrir el correo desde el móvil, y cada navegador tenía la suya. Ahora es un
 * valor por CUENTA en el servidor.
 *
 * GET lee; POST guarda (el checkOrigin CSRF de Astro rechaza el PUT del
 * navegador, así que el BFF traduce POST → PUT, igual que /api/vacation). El
 * tope de tamaño lo impone el servicio: aquí se propaga su 400 tal cual para que
 * el cliente pueda avisar en vez de dar por guardado algo que no se guardó.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailOrigin } from '../../lib/mailrest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/signature');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { signature?: string };
    return json({ signature: body.signature ?? '' });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as { signature?: string };
  try {
    const res = await fetch(`${mailOrigin()}/api/v1/signature`, {
      method: 'PUT',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify({ signature: body.signature ?? '' }),
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
