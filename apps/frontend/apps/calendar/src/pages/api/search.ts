/**
 * GET /api/search?q=&from=&to=&calendar=&cursor=&limit= — búsqueda server-side de
 * eventos del servicio Go Sytue Calendar (/api/v1/events/search, Bearer de Sytue
 * ID). Proxy BFF autorizado por cookie: el access token httpOnly nunca llega al
 * navegador. Solo proxy — la búsqueda (FTS/trigram, rango, keyset, aislamiento por
 * dueño) vive en Go. Es una LECTURA (GET): sin CSRF.
 */
import type { APIRoute } from 'astro';
import { consulta } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL, fallo, forwardError, json } from '../../lib/caldav.js';

export const prerender = false;

const buscar: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  // Reenvía la query verbatim; el servicio Go valida cada parámetro, acota el
  // límite y aísla por dueño (solo los calendarios del usuario autenticado).
  try {
    const res = await fetch(
      `${CALENDAR_BASE_URL}/api/v1/events/search?${await consulta(ctx.request, ctx.url)}`,
      {
        headers: { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' },
      },
    );
    if (!res.ok) return forwardError(res);
    return json(await res.json());
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};

// GET y QUERY, el mismo handler. QUERY es el preferente del cliente y GET el
// respaldo para quien no lo hable; ver `consulta` en @sytue/auth/server.
export const GET: APIRoute = buscar;
export const QUERY: APIRoute = buscar;
