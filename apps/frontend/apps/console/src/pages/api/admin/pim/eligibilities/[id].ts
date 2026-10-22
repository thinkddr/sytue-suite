/*
 * BFF de Sytue Admin — PIM: una elegibilidad (admin).
 *
 *   DELETE /api/admin/pim/eligibilities/:id   Retira la elegibilidad.
 *
 * Proxya a Sytue ID (DELETE /api/v1/admin/pim/eligibilities/:id, Go) con el access token
 * httpOnly; exige rol admin/owner. Propaga el status del servicio (403/404).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { deletePimEligibility, proxyStatus } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function guardStatus(s: number): Response {
  return json({ error: s === 403 ? 'forbidden' : 'unauthenticated' }, s === 403 ? 403 : 401);
}

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    await deletePimEligibility(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
