/**
 * GET /api/calendar-export?id=<uuid> — «Descargar .ics» del menú de un
 * calendario (servicio Go Sytue Calendar, /api/v1/calendars/{id}/export.ics,
 * Bearer de Sytue ID). Proxy BFF autorizado por cookie: el access token
 * httpOnly nunca llega al navegador.
 *
 * Enlace navegable (<a href>) y no una llamada `fetch` + blob: es una
 * descarga de verdad, y el navegador ya sabe hacerla — con `fetch` habría que
 * leer el cuerpo entero en memoria para volver a ofrecerlo como blob, que es
 * peor para un calendario grande y no aporta nada aquí.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL, fallo, forwardError, json } from '../../lib/caldav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = ctx.url.searchParams.get('id');
  if (!id) return json({ error: 'missing_id' }, 400);

  try {
    const res = await fetch(
      `${CALENDAR_BASE_URL}/api/v1/calendars/${encodeURIComponent(id)}/export.ics`,
      {
        headers: { authorization: `Bearer ${session.accessToken}` },
      },
    );
    if (!res.ok) return forwardError(res);
    // Se reenvían tal cual: Content-Type y Content-Disposition (con el nombre del
    // calendario) los pone el servicio Go, que es quien conoce el nombre.
    const headers = new Headers();
    headers.set('content-type', res.headers.get('content-type') ?? 'text/calendar; charset=utf-8');
    const disposition = res.headers.get('content-disposition');
    if (disposition) headers.set('content-disposition', disposition);
    headers.set('cache-control', 'no-store');
    return new Response(res.body, { status: 200, headers });
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};
