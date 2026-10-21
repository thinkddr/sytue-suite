/**
 * /api/labels — CRUD de etiquetas + aplicación en lote, proxy BFF al servicio Go
 * (REST /api/v1/labels…). GET lista; POST multiplexa por la forma del cuerpo, que
 * es el patrón del resto de la app para no depender de PUT/DELETE (el checkOrigin
 * CSRF de Astro los rechaza desde el navegador):
 *   { name, color }                         → crear
 *   { id, name, color }                     → renombrar/recolorear
 *   { id, delete:true }                     → borrar (no borra mensajes)
 *   { id, messageIds:[…], add:boolean }     → aplicar/quitar a un lote (un hilo)
 * Autorizado por cookie de sesión; la autorización por dueño la impone el servicio.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost, mailOrigin } from '../../lib/mailrest.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;

interface RawLabel {
  id: string;
  name?: string;
  color?: string;
  total_emails?: number;
  unread_emails?: number;
}

/** Normaliza la etiqueta del Go (snake_case) a la forma camelCase del cliente. */
function mapLabel(l: RawLabel) {
  return {
    id: l.id,
    name: l.name ?? '',
    color: l.color ?? '',
    totalEmails: l.total_emails ?? 0,
    unreadEmails: l.unread_emails ?? 0,
  };
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/labels');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { labels?: RawLabel[] };
    return json({ labels: (body.labels ?? []).map(mapLabel) });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

interface Payload {
  id?: string;
  name?: string;
  color?: string;
  delete?: boolean;
  messageIds?: string[];
  add?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  const token = session.accessToken;

  try {
    // Borrar.
    if (body.id && body.delete === true) {
      if (!UUID.test(body.id)) return json({ error: 'invalid_id' }, 400);
      const res = await fetchAuthed(token, `DELETE`, `/api/v1/labels/${body.id}`);
      if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
      return json({ ok: true });
    }
    // Aplicar/quitar a un lote (un hilo).
    if (body.id && Array.isArray(body.messageIds)) {
      if (!UUID.test(body.id)) return json({ error: 'invalid_id' }, 400);
      const res = await mailPost(token, `/api/v1/labels/${body.id}/apply`, {
        message_ids: body.messageIds,
        add: body.add === true,
      });
      if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
      return json({ ok: true });
    }
    // Renombrar/recolorear (id + name).
    if (body.id && typeof body.name === 'string') {
      if (!UUID.test(body.id)) return json({ error: 'invalid_id' }, 400);
      const res = await fetchAuthed(token, `PUT`, `/api/v1/labels/${body.id}`, {
        name: body.name,
        color: body.color ?? '',
      });
      if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
      return json({ ok: true });
    }
    // Crear (name sin id).
    if (typeof body.name === 'string') {
      const res = await mailPost(token, `/api/v1/labels`, {
        name: body.name,
        color: body.color ?? '',
      });
      if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
      return json({ label: mapLabel((await res.json()) as RawLabel) }, 201);
    }
    return json({ error: 'invalid_payload' }, 400);
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

/** PUT/DELETE autorizado (Bearer) al servicio Go; el navegador nunca los emite. */
function fetchAuthed(
  token: string,
  method: 'PUT' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<Response> {
  return fetch(`${mailOrigin()}${path}`, {
    method,
    headers: {
      authorization: `Bearer ${token}`,
      accept: 'application/json',
      ...(body ? { 'content-type': 'application/json' } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  });
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
