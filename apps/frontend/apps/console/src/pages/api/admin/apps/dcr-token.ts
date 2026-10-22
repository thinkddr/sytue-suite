/*
 * BFF de Sytue Admin — token de registro dinámico de apps (RFC 7591).
 *
 *   POST /api/admin/apps/dcr-token -> { initial_access_token, registration_endpoint, expires_at }
 *
 * El token viaja UNA vez: Sytue ID guarda solo su hash. Este proxy no lo
 * registra ni lo cachea por lo mismo — lo que no se guarda no se filtra.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { createDCRToken, proxyStatus } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    // no-store también aquí: un token de registro en la caché del navegador es
    // un token que sobrevive a cerrar la pestaña.
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  try {
    return json(await createDCRToken(guard.session), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
