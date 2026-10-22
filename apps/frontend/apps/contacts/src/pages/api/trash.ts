/**
 * Papelera — proxy BFF autorizado por cookie hacia la API REST del servicio Go
 * Sytue Contacts. El borrado suave YA existe (DELETE de un contacto); aquí se
 * expone LISTAR la papelera, RESTAURAR y ELIMINAR DEFINITIVAMENTE.
 *
 * GET  /api/trash                       -> GET    /api/v1/trash          { items }
 * POST /api/trash { id, action }        -> restaurar | purgar
 *   action = 'restore' -> POST   /api/v1/contacts/{id}/restore
 *   action = 'purge'   -> DELETE /api/v1/trash/{id}
 *
 * Ambas mutaciones van por POST (Astro rechaza DELETE/PUT con cuerpo desde el
 * navegador). El servicio Go solo actúa sobre la papelera del propio dueño.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { restJson, RestError } from '../../lib/rest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const body = await restJson<{ items: unknown[] }>(session.accessToken, 'GET', '/api/v1/trash');
    return json(body);
  } catch (err) {
    return fail(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { id?: string; action?: string };
  const id = body.id?.trim();
  if (!id) return json({ error: 'missing_id' }, 400);

  try {
    if (body.action === 'purge') {
      await restJson<void>(
        session.accessToken,
        'DELETE',
        `/api/v1/trash/${encodeURIComponent(id)}`,
      );
    } else if (body.action === 'restore') {
      await restJson<void>(
        session.accessToken,
        'POST',
        `/api/v1/contacts/${encodeURIComponent(id)}/restore`,
      );
    } else {
      return json({ error: 'invalid_action' }, 400);
    }
    return json({ ok: true });
  } catch (err) {
    return fail(err);
  }
};

function fail(err: unknown): Response {
  if (err instanceof RestError)
    return json({ error: 'contacts_error', message: err.message }, err.status);
  return json({ error: 'contacts_error', message: String(err) }, 502);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
