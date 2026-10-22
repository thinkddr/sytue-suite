/*
 * BFF de Sytue Admin — marcar de confianza el dispositivo actual (self-service).
 *
 *   POST /api/me/devices/:id/trust   Marca de confianza el dispositivo actual.
 *
 * Proxya a Sytue ID (POST /api/v1/me/devices/:id/trust, Go) con el access token httpOnly
 * y REENVÍA la cookie de dispositivo firmada: el servicio exige que :id sea el
 * dispositivo desde el que se accede (prueba de posesión). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession, deviceCookieHeader } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { proxyStatus, trustMyDevice } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);
  const id = ctx.params.id ?? '';
  const deviceCookie = deviceCookieHeader((n) => ctx.cookies.get(n)?.value);
  try {
    return json(await trustMyDevice(guard.session, id, deviceCookie));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
