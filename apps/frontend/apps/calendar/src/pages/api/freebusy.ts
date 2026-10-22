/**
 * GET /api/freebusy — «buscar hueco»: huecos libres comunes de los participantes.
 * Proxy BFF autorizado por cookie contra la API REST del servicio Go Sytue
 * Calendar (/api/v1/freebusy/suggest, Bearer de Sytue ID). Solo proxy: el calculo
 * (interseccion de ocupados, franja laboral, DST) vive en Go, que ademas incluye
 * SIEMPRE al organizador autenticado. Es una LECTURA (GET): sin CSRF.
 *
 * Query: start, end (RFC3339), duration (min), tz (IANA), work_start/work_end
 * (HH:MM), weekdays (true), user (sub, repetible).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL, fallo, forwardError, json } from '../../lib/caldav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  // Reenvia la query verbatim; el servicio Go valida cada parametro y acota la
  // ventana/participantes. Los `user` son subs del mismo tenant (el free/busy
  // respeta RLS y solo expone ocupado/libre, nunca detalles de eventos ajenos).
  try {
    const res = await fetch(`${CALENDAR_BASE_URL}/api/v1/freebusy/suggest${ctx.url.search}`, {
      headers: { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' },
    });
    if (!res.ok) return forwardError(res);
    return json(await res.json());
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};
