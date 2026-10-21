/**
 * /api/shared — «Compartido conmigo»: los calendarios que otros usuarios del
 * tenant han compartido con el usuario (por su correo/sub verificado del JWT).
 * Proxy BFF autorizado por cookie contra la API REST del servicio Go Sytue
 * Calendar (/api/v1/calendars/shared-with-me). Solo proxy: la verja (deny-by-
 * default) y el casado del destinatario viven en Go.
 *
 * GET → { calendars: (Calendar & { shared, permission, sharedBy })[] }
 *
 * Cada calendario compartido se pinta reutilizando el mismo camino CalDAV que los
 * propios: su URL de colección es la del DUEÑO (/dav/calendars/<owner_sub>/<name>/),
 * que el backend resuelve por ruta dentro del tenant. `readOnly` es true si el
 * permiso es «reader» (o el calendario es una suscripción), de modo que la rejilla
 * bloquea arrastrar/editar igual que un calendario ICS; un «writer» es editable.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL, fallo, forwardError, json } from '../../lib/caldav.js';
import { toSharedClient, type SharedRestCalendar } from '../../lib/restCalendar.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await fetch(`${CALENDAR_BASE_URL}/api/v1/calendars/shared-with-me`, {
      headers: { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' },
    });
    if (!res.ok) return forwardError(res);
    const { calendars } = (await res.json()) as { calendars: SharedRestCalendar[] };
    return json({ calendars: (calendars ?? []).map(toSharedClient) });
  } catch (err) {
    return fallo(err, 'shared_error');
  }
};
