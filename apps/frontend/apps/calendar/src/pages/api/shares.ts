/**
 * /api/shares — gestión de comparticiones de un calendario PROPIO (compartir con
 * ACL). Proxy BFF autorizado por cookie contra la API REST del servicio Go Sytue
 * Calendar. Solo proxy: la verja «solo el dueño» y la validación del correo/permiso
 * viven en Go (que casa el destinatario contra el email/sub verificado del JWT).
 *
 * GET  ?calendar=<id>                         → { shares } de ese calendario (dueño)
 * POST { calendar, email, permission }        → comparte (reader|writer)
 *      { calendar, shareId, delete: true }    → revoca un share
 * (Mutaciones siempre por POST: el CSRF de Astro rechaza otros verbos con cuerpo.)
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { fallo, forwardError, json, rest } from '../../lib/caldav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const calendar = ctx.url.searchParams.get('calendar');
  if (!calendar) return json({ error: 'missing_calendar' }, 400);
  try {
    const res = await rest(
      session.accessToken,
      'GET',
      `/calendars/${encodeURIComponent(calendar)}/shares`,
    );
    if (!res.ok) return forwardError(res);
    return json((await res.json()) as unknown);
  } catch (err) {
    return fallo(err, 'shares_error');
  }
};

interface PostPayload {
  calendar?: string;
  email?: string;
  permission?: string;
  shareId?: string;
  delete?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as PostPayload;
  const calendar = body.calendar?.trim();
  if (!calendar) return json({ error: 'missing_calendar' }, 400);

  try {
    // Revocar un share.
    if (body.delete) {
      if (!body.shareId) return json({ error: 'missing_share_id' }, 400);
      const res = await rest(
        session.accessToken,
        'DELETE',
        `/calendars/${encodeURIComponent(calendar)}/shares/${encodeURIComponent(body.shareId)}`,
      );
      if (!res.ok) return forwardError(res);
      return json({ ok: true });
    }

    // Compartir (reader|writer) o delegar la agenda (manage).
    const email = body.email?.trim();
    if (!email) return json({ error: 'missing_email' }, 400);
    if (
      body.permission !== 'reader' &&
      body.permission !== 'writer' &&
      body.permission !== 'manage'
    ) {
      return json({ error: 'invalid_permission' }, 400);
    }
    const res = await rest(
      session.accessToken,
      'POST',
      `/calendars/${encodeURIComponent(calendar)}/shares`,
      { email, permission: body.permission },
    );
    if (!res.ok) return forwardError(res);
    return json({ share: (await res.json()) as unknown }, 201);
  } catch (err) {
    return fallo(err, 'shares_error');
  }
};
