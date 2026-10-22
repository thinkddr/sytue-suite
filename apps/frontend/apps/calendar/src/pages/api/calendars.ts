/**
 * /api/calendars — gestion de calendarios del usuario (multi-calendario).
 * Proxy BFF autorizado por cookie contra la API REST del servicio Go Sytue
 * Calendar (/api/v1/calendars, Bearer de Sytue ID). Solo proxy: la logica
 * (nombre unico, default inmutable, validacion de color) vive en Go.
 *
 * GET  -> lista { calendars } con id/url/displayName/color/isDefault.
 * POST -> { displayName, color }            crea
 *         { id, displayName?, color? }      renombra / recolorea (PATCH en Go)
 *         { id, delete: true }              borra (DELETE en Go; el default da 403)
 * (Mutaciones siempre por POST: el CSRF de Astro rechaza DELETE/PATCH con cuerpo.)
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { fallo, forwardError, json, rest } from '../../lib/caldav.js';
import { toClient, type RestCalendar } from '../../lib/restCalendar.js';

export const prerender = false;

/**
 * name = segmento de ruta CalDAV derivado del nombre visible: minusculas ASCII
 * y guiones (URL-safe garantizado para las URLs sintetizadas por calDavUrl).
 */
function slugify(s: string): string {
  const slug = s
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64);
  return slug || `cal-${Date.now().toString(36)}`;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await rest(session.accessToken, 'GET', '/calendars');
    if (!res.ok) return forwardError(res);
    const { calendars } = (await res.json()) as { calendars: RestCalendar[] };
    return json({ calendars: calendars.map((c) => toClient(session.user.id, c)) });
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};

interface PostPayload {
  id?: string;
  displayName?: string;
  color?: string;
  delete?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as PostPayload;
  if (body.color !== undefined && !/^#[0-9A-Fa-f]{6}$/.test(body.color)) {
    return json({ error: 'invalid_color' }, 400);
  }

  try {
    // Borrado.
    if (body.delete) {
      if (!body.id) return json({ error: 'missing_id' }, 400);
      const res = await rest(
        session.accessToken,
        'DELETE',
        `/calendars/${encodeURIComponent(body.id)}`,
      );
      if (!res.ok) return forwardError(res);
      return json({ ok: true });
    }

    // Renombrar / cambiar color.
    if (body.id) {
      if (body.displayName !== undefined && !body.displayName.trim()) {
        return json({ error: 'invalid_display_name' }, 400);
      }
      const res = await rest(
        session.accessToken,
        'PATCH',
        `/calendars/${encodeURIComponent(body.id)}`,
        {
          ...(body.displayName !== undefined ? { display_name: body.displayName.trim() } : {}),
          ...(body.color !== undefined ? { color: body.color } : {}),
        },
      );
      if (!res.ok) return forwardError(res);
      return json({ calendar: toClient(session.user.id, (await res.json()) as RestCalendar) });
    }

    // Crear.
    const displayName = body.displayName?.trim();
    if (!displayName) return json({ error: 'missing_display_name' }, 400);
    const payload = (name: string) => ({
      name,
      display_name: displayName,
      ...(body.color ? { color: body.color } : {}),
    });
    let res = await rest(session.accessToken, 'POST', '/calendars', payload(slugify(displayName)));
    if (res.status === 409) {
      // Colision de slug (nombre visible repetido): sufijo unico y reintento.
      res = await rest(
        session.accessToken,
        'POST',
        '/calendars',
        payload(`${slugify(displayName)}-${Date.now().toString(36)}`),
      );
    }
    if (!res.ok) return forwardError(res);
    return json({ calendar: toClient(session.user.id, (await res.json()) as RestCalendar) }, 201);
  } catch (err) {
    return fallo(err, 'calendar_error');
  }
};
