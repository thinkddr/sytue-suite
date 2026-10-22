/*
 * BFF de Sytue Admin — revocar uno de mis dispositivos (self-service).
 *
 *   DELETE /api/me/devices/:id   Revoca (borra) el dispositivo; deja de reconocerse.
 *
 * Proxya a Sytue ID (DELETE /api/v1/me/devices/:id, Go) con el access token httpOnly.
 * Sytue ID acota por usuario (un dispositivo ajeno ⇒ 404).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { proxyStatus, revokeMyDevice } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);
  const id = ctx.params.id ?? '';
  try {
    await revokeMyDevice(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
