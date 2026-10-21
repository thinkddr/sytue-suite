/**
 * LA AGENDA DEL DÍA, leída de Sytue Calendar con el token de quien mira.
 *
 * GET /api/agenda?date=AAAA-MM-DD&from=&to= — los eventos de ese día. `from` y `to`
 * son la medianoche local del navegador en RFC 3339: el día civil lo decide su reloj,
 * como el diario. Un evento de día completo es FLOTANTE (el servicio lo guarda a las
 * 00:00 UTC) y la ventana local lo pillaría también la víspera, así que esos se
 * filtran por su fecha y no por la ventana.
 *
 * POST /api/agenda { date, summary } — crea un evento de día completo en el
 * calendario por defecto y devuelve el enlace que la nota guarda (`calendar_event`
 * con el UID, como el selector de `/api/suite`).
 *
 * Sin Calendar desplegado, la agenda sale vacía: el diario no se rompe por una app
 * opcional de la suite.
 */
import type { APIRoute } from 'astro';
import { CalendarClient, bearer } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { calendarApiUrl, cuerpo, fallo, json } from '../../lib/notes.js';
import { deCalendar } from '../../lib/elegibles.js';
import { eventoDelDia, eventoDeDiaCompleto } from '../../client/agenda.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const q = new URL(ctx.request.url).searchParams;
  const date = q.get('date') ?? '';
  const from = q.get('from') ?? '';
  const to = q.get('to') ?? '';
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !from || !to) return json({ error: 'bad_request' }, 400);
  const client = new CalendarClient({
    baseUrl: calendarApiUrl(),
    auth: bearer(session.accessToken),
  });
  try {
    const events = await client.listEvents({ from, to });
    return json({
      events: events
        .filter((e) => eventoDelDia(e, date))
        .map((e) => ({
          ...deCalendar(e, session.user.tenant),
          start: e.start,
          ...(e.end ? { end: e.end } : {}),
          ...(e.allDay ? { allDay: true } : {}),
        })),
    });
  } catch {
    return json({ events: [] });
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const input = await cuerpo<{ date?: string; summary?: string }>(ctx);
  const summary = input.summary?.trim().slice(0, 500) ?? '';
  const evento = eventoDeDiaCompleto(input.date ?? '', summary);
  if (!evento) return json({ error: 'bad_request' }, 400);
  const client = new CalendarClient({
    baseUrl: calendarApiUrl(),
    auth: bearer(session.accessToken),
  });
  try {
    const calendarios = await client.listCalendars();
    const destino = calendarios.find((c) => c.isDefault) ?? calendarios.find((c) => !c.readOnly);
    if (!destino) return json({ error: 'no_calendar' }, 409);
    const e = await client.createEvent({ calendarId: destino.id, ...evento });
    const el = deCalendar(e, session.user.tenant);
    return json({
      link: { target_kind: 'calendar_event', target_id: el.id, label: el.name, url: el.url },
    });
  } catch (err) {
    return fallo(err);
  }
};
