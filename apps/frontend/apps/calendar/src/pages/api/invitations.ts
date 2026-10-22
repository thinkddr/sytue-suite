/**
 * Bandeja de planificacion (scheduling inbox iTIP, RFC 6638) del servicio Go
 * Sytue Calendar.
 *
 *   GET  /api/invitations?pending=true  -> lista las invitaciones del usuario.
 *   POST /api/invitations               -> responde: { id, status }
 *                                          status = accepted | declined | tentative
 *
 * Proxy BFF autorizado por cookie: el access token httpOnly autoriza la API REST
 * (/api/v1) del servicio Go; nunca llega al navegador. A diferencia de los
 * eventos (CalDAV), la bandeja es REST JSON pura.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL, fallo, json } from '../../lib/caldav.js';

export const prerender = false;

interface Invitation {
  id: string;
  event_id?: string;
  uid: string;
  method: string;
  organizer?: string;
  recipient: string;
  status: string;
  created_at: string;
  responded_at?: string;
  /** Enriquecido por el BFF desde el evento asociado (best-effort). */
  summary?: string;
  start?: string;
}

const REST = `${CALENDAR_BASE_URL.replace(/\/$/, '')}/api/v1`;

const auth = (token: string) => ({ authorization: `Bearer ${token}`, accept: 'application/json' });

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const pending = ctx.url.searchParams.get('pending') === 'true';
  try {
    const res = await fetch(`${REST}/invitations${pending ? '?pending=true' : ''}`, {
      headers: auth(session.accessToken),
    });
    if (!res.ok)
      return json(
        { error: 'calendar_error', status: res.status },
        res.status < 500 ? res.status : 502,
      );
    const { invitations } = (await res.json()) as { invitations: Invitation[] };

    // Enriquece con el titulo/inicio del evento para pintar la bandeja. N+1 sobre
    // la bandeja (pocas invitaciones); best-effort: si el evento no es legible,
    // se muestra solo el organizador. ponytail: N+1, batch si la bandeja crece.
    await Promise.all(
      invitations.map(async (inv) => {
        if (!inv.event_id) return;
        const ev = await fetch(`${REST}/events/${encodeURIComponent(inv.event_id)}`, {
          headers: auth(session.accessToken),
        }).catch(() => null);
        if (ev?.ok) {
          const e = (await ev.json()) as { summary?: string; dtstart?: string };
          if (e.summary) inv.summary = e.summary;
          if (e.dtstart) inv.start = e.dtstart;
        }
      }),
    );
    return json({ invitations });
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { id?: string; status?: string };
  if (!body.id || !body.status) return json({ error: 'invalid_payload' }, 400);

  try {
    const res = await fetch(`${REST}/invitations/${encodeURIComponent(body.id)}/respond`, {
      method: 'POST',
      headers: { ...auth(session.accessToken), 'content-type': 'application/json' },
      body: JSON.stringify({ status: body.status }),
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
