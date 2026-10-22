/*
 * BFF de Sytue Admin — reenviar la verificación del propio email.
 *
 *   POST /api/me/email/verify   Envía (o reenvía) el enlace de verificación a la
 *       dirección de la cuenta del usuario autenticado. Sytue ID emite un token de
 *       un solo uso y lo manda por email (rate-limitado en el servicio).
 *
 * Proxya a Sytue ID (POST /api/v1/me/email/verify/send, Go) con el access token
 * httpOnly. Cualquier sesión válida sirve (verificar el correo propio no exige rol
 * admin), pero la Console ya solo la usan admin/owner.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { proxyStatus, resendEmailVerification } from '../../../../lib/admin.js';

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
  try {
    return json(await resendEmailVerification(guard.session));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
