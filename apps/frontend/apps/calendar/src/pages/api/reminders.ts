/**
 * Recordatorios in-app (VALARM) del servicio Go Sytue Calendar.
 *
 *   GET  /api/reminders?cursor=  -> recordatorios vencidos del usuario (keyset).
 *   POST /api/reminders          -> descarta: { id }  (mutacion por POST, CSRF-safe)
 *                                   o pospone: { id, snooze: minutos } (1–1440)
 *
 * Proxy BFF autorizado por cookie: el access token httpOnly autoriza la API REST
 * (/api/v1) del servicio Go; nunca llega al navegador. REST JSON puro.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL, fallo, json } from '../../lib/caldav.js';

export const prerender = false;

const REST = `${CALENDAR_BASE_URL.replace(/\/$/, '')}/api/v1`;
const auth = (token: string) => ({ authorization: `Bearer ${token}`, accept: 'application/json' });

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = new URLSearchParams();
  const cursor = ctx.url.searchParams.get('cursor');
  if (cursor) params.set('cursor', cursor);
  const limit = ctx.url.searchParams.get('limit');
  if (limit) params.set('limit', limit);

  try {
    const qs = params.toString();
    const res = await fetch(`${REST}/reminders/due${qs ? `?${qs}` : ''}`, {
      headers: auth(session.accessToken),
    });
    if (!res.ok)
      return json(
        { error: 'calendar_error', status: res.status },
        res.status < 500 ? res.status : 502,
      );
    return json(await res.json());
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { id?: string; snooze?: unknown };
  if (!body.id) return json({ error: 'missing_id' }, 400);
  // Posponer viaja por la misma ruta: solo un entero del rango que admite el servicio.
  const posponer = body.snooze;
  if (
    posponer !== undefined &&
    !(
      typeof posponer === 'number' &&
      Number.isInteger(posponer) &&
      posponer >= 1 &&
      posponer <= 1440
    )
  )
    return json({ error: 'invalid_snooze' }, 400);

  try {
    const id = encodeURIComponent(body.id);
    const res =
      posponer === undefined
        ? await fetch(`${REST}/reminders/${id}/dismiss`, {
            method: 'POST',
            headers: auth(session.accessToken),
          })
        : await fetch(`${REST}/reminders/${id}/snooze`, {
            method: 'POST',
            headers: { ...auth(session.accessToken), 'content-type': 'application/json' },
            body: JSON.stringify({ minutes: posponer }),
          });
    if (!res.ok)
      return json(
        { error: 'calendar_error', status: res.status },
        res.status < 500 ? res.status : 502,
      );
    if (posponer !== undefined) {
      const { trigger_at } = (await res.json()) as { trigger_at?: string };
      return json({ ok: true, trigger_at });
    }
    return json({ ok: true });
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};
