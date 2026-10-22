/**
 * Recordatorios in-app (VALARM) del servicio Go Sytue Calendar.
 *
 *   GET  /api/reminders?cursor=  -> recordatorios vencidos del usuario (keyset).
 *   POST /api/reminders          -> descarta: { id }  (mutacion por POST, CSRF-safe)
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

  const body = (await ctx.request.json().catch(() => ({}))) as { id?: string };
  if (!body.id) return json({ error: 'missing_id' }, 400);

  try {
    const res = await fetch(`${REST}/reminders/${encodeURIComponent(body.id)}/dismiss`, {
      method: 'POST',
      headers: auth(session.accessToken),
    });
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
