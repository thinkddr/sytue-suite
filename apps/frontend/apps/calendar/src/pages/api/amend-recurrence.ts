/**
 * Recorte por alcance del maestro de una serie recurrente (servicio Go Sytue
 * Calendar): al editar/borrar «solo este» / «este y siguientes» de una serie, el
 * backend añade EXDATE o fija UNTIL sin corromperla. La instancia/serie nueva la
 * crea el cliente aparte por el PUT CalDAV habitual (/api/event).
 *
 *   POST /api/amend-recurrence  -> { calendarId, uid, recurrenceId, mode, etag? }
 *                                  mode = 'exclude' | 'until'
 *
 * Proxy BFF autorizado por cookie. Mutacion por POST (CSRF-safe).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL, fallo, json } from '../../lib/caldav.js';

export const prerender = false;

const REST = `${CALENDAR_BASE_URL.replace(/\/$/, '')}/api/v1`;

interface Payload {
  calendarId?: string;
  uid?: string;
  recurrenceId?: string;
  mode?: string;
  etag?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (!body.calendarId || !body.uid || !body.recurrenceId) {
    return json({ error: 'invalid_payload' }, 400);
  }
  if (body.mode !== 'exclude' && body.mode !== 'until') {
    return json({ error: 'invalid_mode' }, 400);
  }

  try {
    const res = await fetch(
      `${REST}/calendars/${encodeURIComponent(body.calendarId)}/amend-recurrence`,
      {
        method: 'POST',
        headers: {
          authorization: `Bearer ${session.accessToken}`,
          accept: 'application/json',
          'content-type': 'application/json',
        },
        body: JSON.stringify({
          uid: body.uid,
          recurrence_id: body.recurrenceId,
          mode: body.mode,
          ...(body.etag ? { etag: body.etag } : {}),
        }),
      },
    );
    if (!res.ok)
      return json(
        { error: 'calendar_error', status: res.status },
        res.status < 500 ? res.status : 502,
      );
    return json({ ok: true });
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};
