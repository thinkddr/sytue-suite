/*
 * BFF de Sytue Admin — mis dispositivos de confianza (self-service).
 *
 *   GET /api/me/devices   Mis dispositivos, con el actual marcado.
 *
 * Proxya a Sytue ID (GET /api/v1/me/devices, Go) con el access token httpOnly y REENVÍA
 * la cookie de dispositivo (httpOnly, firmada) del navegador para que el servicio marque
 * cuál es el dispositivo actual. Cualquier sesión válida basta. El token nunca sale al
 * cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession, deviceCookieHeader } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadMyDevices } from '../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);
  const deviceCookie = deviceCookieHeader((n) => ctx.cookies.get(n)?.value);
  const { live, data } = await loadMyDevices(guard.session, deviceCookie);
  return json({ live, devices: data });
};
