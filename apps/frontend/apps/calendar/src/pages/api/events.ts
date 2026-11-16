/**
 * GET /api/events?calendarUrl=&calendarId=&start=&end= — eventos de un calendario
 * en un rango, una entrada por OCURRENCIA. start/end en formato iCalendar UTC
 * (yyyymmddThhmmssZ). Proxy BFF autorizado por cookie.
 *
 * Con `calendarId` y rango, las fechas las calcula el servicio
 * (`/calendars/{id}/instances`, el motor de Fyne) y el `REPORT` CalDAV pone el
 * href/etag/iCal de cada maestro para poder editar: ver `lib/instancias.ts`. Sin
 * ellos sale el `REPORT` tal cual (los maestros, sin expandir).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { calDavClient, fallo, json, rest, rutaDav } from '../../lib/caldav.js';
import {
  ocurrenciasDeInstancias,
  rfc3339DeIcal,
  type InstanciaServicio,
} from '../../lib/instancias.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const calendarUrl = rutaDav(ctx.url.searchParams.get('calendarUrl'));
  if (!calendarUrl) return json({ error: 'missing_calendarUrl' }, 400);
  const start = ctx.url.searchParams.get('start') ?? undefined;
  const end = ctx.url.searchParams.get('end') ?? undefined;
  const calendarId = ctx.url.searchParams.get('calendarId') ?? '';
  const desde = start ? rfc3339DeIcal(start) : null;
  const hasta = end ? rfc3339DeIcal(end) : null;

  try {
    const client = calDavClient(session.accessToken, session.user.id);
    const maestros = client.listEvents(
      calendarUrl,
      start ? { start, ...(end ? { end } : {}) } : undefined,
    );
    if (!calendarId || !desde || !hasta) return json({ events: await maestros });

    const query = new URLSearchParams({ start: desde, end: hasta });
    const [lista, res] = await Promise.all([
      maestros,
      rest(
        session.accessToken,
        'GET',
        `/calendars/${encodeURIComponent(calendarId)}/instances?${query.toString()}`,
      ),
    ]);
    // El mismo criterio que el REPORT (`fallo`): un 4xx tal cual —un 403 es «ya no
    // tienes acceso» y la vista lo aparta— y 502 para lo demás.
    if (!res.ok)
      return json(
        { error: 'calendar_error' },
        res.status >= 400 && res.status < 500 ? res.status : 502,
      );
    const { instances } = (await res.json()) as { instances?: InstanciaServicio[] };
    return json({ events: ocurrenciasDeInstancias(lista, instances ?? []) });
  } catch (err) {
    return fallo(err, 'caldav_error');
  }
};
