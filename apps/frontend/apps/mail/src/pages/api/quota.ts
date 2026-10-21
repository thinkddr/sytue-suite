/**
 * /api/quota — uso de almacenamiento de la cuenta (indicador del sidebar), proxy
 * BFF de solo lectura al servicio Go (REST GET /api/v1/quota). Devuelve los bytes
 * usados y la cuota total; el uso es el contador agregado que el servicio mantiene
 * (sin N+1) y es owner-scoped (solo la cuota del propio usuario). Autorizado por
 * cookie de sesión; el access token nunca toca el navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet } from '../../lib/mailrest.js';

export const prerender = false;

interface RawQuota {
  used_bytes?: number;
  quota_bytes?: number;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/quota');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const q = (await res.json()) as RawQuota;
    return json({ usedBytes: q.used_bytes ?? 0, quotaBytes: q.quota_bytes ?? 0 });
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
