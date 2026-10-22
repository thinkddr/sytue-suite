/**
 * /api/resources — recursos reservables (salas, equipos) del tenant. Proxy BFF
 * autorizado por cookie contra la API REST del servicio Go Sytue Calendar. Solo
 * proxy: el anti doble-reserva atómico, el aislamiento por tenant y la verja de
 * rol/gestor viven en Go.
 *
 * GET  ?start&end (RFC3339, opcional)     → { resources } (con disponibilidad si hay rango)
 * POST { action: 'create', ...campos }    → alta de recurso (solo admin/owner)
 *      { action: 'update', id, ...patch } → edición (gestor o admin)
 *      { action: 'delete', id }           → baja (gestor o admin)
 *      { action: 'book', id, uid, ... }   → reservar → { status: accepted|declined|pending }
 *      { action: 'release', id, source_uid } → liberar
 * (Mutaciones siempre por POST: el CSRF de Astro rechaza otros verbos con cuerpo.)
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { fallo, forwardError, json, rest } from '../../lib/caldav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await rest(session.accessToken, 'GET', `/resources${ctx.url.search}`);
    if (!res.ok) return forwardError(res);
    return json((await res.json()) as unknown);
  } catch (err) {
    return fallo(err, 'resources_error');
  }
};

interface PostPayload {
  action?: string;
  id?: string;
  source_uid?: string;
  [k: string]: unknown;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as PostPayload;
  const token = session.accessToken;
  const { action, id, source_uid, ...rest_ } = body;

  try {
    switch (action) {
      case 'create': {
        const res = await rest(token, 'POST', '/resources', rest_);
        if (!res.ok) return forwardError(res);
        return json((await res.json()) as unknown, 201);
      }
      case 'update': {
        if (!id) return json({ error: 'missing_id' }, 400);
        const res = await rest(token, 'PATCH', `/resources/${encodeURIComponent(id)}`, rest_);
        if (!res.ok) return forwardError(res);
        return json((await res.json()) as unknown);
      }
      case 'delete': {
        if (!id) return json({ error: 'missing_id' }, 400);
        const res = await rest(token, 'DELETE', `/resources/${encodeURIComponent(id)}`);
        if (!res.ok) return forwardError(res);
        return json({ ok: true });
      }
      case 'book': {
        if (!id) return json({ error: 'missing_id' }, 400);
        const res = await rest(token, 'POST', `/resources/${encodeURIComponent(id)}/book`, rest_);
        if (!res.ok) return forwardError(res);
        return json((await res.json()) as unknown);
      }
      case 'release': {
        if (!id || !source_uid) return json({ error: 'missing_id_or_uid' }, 400);
        const res = await rest(
          token,
          'DELETE',
          `/resources/${encodeURIComponent(id)}/book?source_uid=${encodeURIComponent(source_uid)}`,
        );
        if (!res.ok) return forwardError(res);
        return json({ ok: true });
      }
      default:
        return json({ error: 'unknown_action' }, 400);
    }
  } catch (err) {
    return fallo(err, 'resources_error');
  }
};
