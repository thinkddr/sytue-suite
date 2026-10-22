/*
 * BFF de Sytue Admin — canales de Rooms de la organización.
 *
 *   PATCH /api/admin/rooms-channels   { id, archived }
 *
 * Solo la escritura: el listado se sirve en el render de la página, que es donde
 * se lee. Un endpoint de lectura aquí solo añadiría un segundo camino a lo mismo.
 *
 * Proxya a `services/rooms` (PATCH /api/v1/admin/channels/{id}) con el access
 * token httpOnly de la sesión. Quién puede lo decide Rooms —`RequireRole
 * ("owner","admin")`— y el aislamiento por organización lo pone RLS con el tenant
 * del token; aquí se exige el mismo rol para no ofrecer un botón que va a acabar
 * en 403.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { setChannelArchived } from '../../../lib/rooms.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
  });
  if (guard instanceof Response) {
    return json({ error: guard.status === 403 ? 'forbidden' : 'unauthenticated' }, guard.status);
  }

  const body = (await ctx.request.json().catch(() => null)) as {
    id?: unknown;
    archived?: unknown;
  } | null;
  if (typeof body?.id !== 'string' || typeof body?.archived !== 'boolean') {
    return json({ error: 'bad_request' }, 400);
  }

  // El código de Rooms se propaga tal cual: un 403 («ya no eres admin») y un 502
  // («Rooms no contesta») llevan a mensajes distintos, y colapsarlos en un 500
  // deja al operador sin saber si el problema es suyo o del servicio.
  const res = await setChannelArchived(guard.session, body.id, body.archived);
  return json(res.ok ? { channel: res.data } : { error: 'upstream' }, res.ok ? 200 : res.status);
};
