/**
 * /api/subscriptions — suscripcion a calendarios ICS externos (el «Añadir
 * calendario por URL» de Google Calendar). Proxy BFF autorizado por cookie contra
 * la API REST del servicio Go Sytue Calendar. Solo proxy: la validacion de la URL,
 * la barrera SSRF, el fetch condicional y el parseo viven en Go.
 *
 * POST -> { url, displayName, color }   crea una suscripcion (calendario read-only)
 *         { id, refresh: true }         fuerza un refresco inmediato («actualizar ahora»)
 * (Mutaciones siempre por POST: el CSRF de Astro rechaza otros verbos con cuerpo.)
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { fallo, forwardError, json, rest } from '../../lib/caldav.js';
import { toClient, type RestCalendar } from '../../lib/restCalendar.js';

export const prerender = false;

interface PostPayload {
  url?: string;
  displayName?: string;
  color?: string;
  id?: string;
  refresh?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as PostPayload;

  try {
    // Refrescar ahora una suscripcion existente.
    if (body.refresh) {
      if (!body.id) return json({ error: 'missing_id' }, 400);
      const res = await rest(
        session.accessToken,
        'POST',
        `/calendars/${encodeURIComponent(body.id)}/refresh`,
      );
      if (!res.ok) return forwardError(res);
      return json({ calendar: toClient(session.user.id, (await res.json()) as RestCalendar) });
    }

    // Crear una suscripcion.
    const url = body.url?.trim();
    if (!url) return json({ error: 'missing_url' }, 400);
    if (!/^https?:\/\//i.test(url)) return json({ error: 'invalid_url' }, 400);
    if (body.color !== undefined && !/^#[0-9A-Fa-f]{6}$/.test(body.color)) {
      return json({ error: 'invalid_color' }, 400);
    }
    const displayName = body.displayName?.trim() || 'Calendario suscrito';
    const res = await rest(session.accessToken, 'POST', '/subscriptions', {
      url,
      display_name: displayName,
      ...(body.color ? { color: body.color } : {}),
    });
    if (!res.ok) return forwardError(res);
    return json({ calendar: toClient(session.user.id, (await res.json()) as RestCalendar) }, 201);
  } catch (err) {
    return fallo(err, 'subscription_error');
  }
};
