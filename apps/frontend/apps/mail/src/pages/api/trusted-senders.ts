/**
 * /api/trusted-senders — remitentes cuyas imágenes remotas se enseñan sin preguntar,
 * por cuenta y en el servidor (REST /api/v1/trusted-senders). GET lista; POST
 * multiplexa por la forma del cuerpo, porque el checkOrigin CSRF de Astro rechaza
 * DELETE desde el navegador:
 *   { address }                 → confiar
 *   { address, remove: true }   → dejar de confiar
 *
 * Las imágenes siguen pasando por nuestro proxy (/api/remote-image) aunque el
 * remitente sea de confianza: confiar quita la pregunta, no la protección de la IP.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailDelete, mailGet, mailPost } from '../../lib/mailrest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/trusted-senders');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { senders?: unknown };
    const senders = Array.isArray(body.senders)
      ? body.senders.filter((s): s is string => typeof s === 'string')
      : [];
    return json({ senders });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as {
    address?: unknown;
    remove?: unknown;
  };
  if (typeof body.address !== 'string' || body.address.trim() === '')
    return json({ error: 'invalid_payload' }, 400);
  const token = session.accessToken;
  try {
    if (body.remove === true) {
      const res = await mailDelete(
        token,
        `/api/v1/trusted-senders?address=${encodeURIComponent(body.address)}`,
      );
      if (!res.ok) return rechazo(res);
      return json({ ok: true });
    }
    const res = await mailPost(token, '/api/v1/trusted-senders', { address: body.address });
    if (!res.ok) return rechazo(res);
    const guardada = (await res.json()) as { address?: string };
    return json({ address: guardada.address ?? body.address.trim().toLowerCase() });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

/** El rechazo del servicio: 400 (no es una dirección) y 409 (tope) se conservan. */
async function rechazo(res: Response): Promise<Response> {
  const sobre = (await res.json().catch(() => ({}))) as { error?: { code?: string } };
  const code = sobre.error?.code;
  const status = [400, 409].includes(res.status) ? res.status : 502;
  return json({ error: 'mail_error', status: res.status, ...(code ? { code } : {}) }, status);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
