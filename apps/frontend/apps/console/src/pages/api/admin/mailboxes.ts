/*
 * BFF de Sytue Admin — direcciones de correo de la organización activa.
 *
 *   GET /api/admin/mailboxes[?user_id=]   Direcciones de la org (o de una persona)
 *                                          más el dominio en el que se reparten.
 *
 * Proxya a Sytue ID (GET /api/v1/admin/mailboxes, Go) con el access token
 * httpOnly; propaga el status del servicio. El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { listMailboxes, proxyStatus } from '../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const userId = ctx.url.searchParams.get('user_id') ?? '';
  try {
    return json(await listMailboxes(guard.session, userId));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
