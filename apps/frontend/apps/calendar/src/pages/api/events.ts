/**
 * GET /api/events?calendarUrl=&start=&end= — eventos de un calendario en un rango
 * (calendar-query REPORT via el SDK CalDAV). start/end en formato iCalendar UTC
 * (yyyymmddThhmmssZ). Proxy BFF autorizado por cookie.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { calDavClient, fallo, json, rutaDav } from '../../lib/caldav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const calendarUrl = rutaDav(ctx.url.searchParams.get('calendarUrl'));
  if (!calendarUrl) return json({ error: 'missing_calendarUrl' }, 400);
  const start = ctx.url.searchParams.get('start') ?? undefined;
  const end = ctx.url.searchParams.get('end') ?? undefined;

  try {
    const client = calDavClient(session.accessToken, session.user.id);
    const events = await client.listEvents(
      calendarUrl,
      start ? { start, ...(end ? { end } : {}) } : undefined,
    );
    return json({ events });
  } catch (err) {
    return fallo(err, 'caldav_error');
  }
};
