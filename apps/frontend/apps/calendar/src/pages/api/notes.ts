/**
 * GET /api/notes?event=<uid> — LAS NOTAS VINCULADAS A UN EVENTO.
 *
 * El parámetro es el UID del evento (RFC 5545), no el identificador interno del
 * servicio: es el que ve la superficie CalDAV y el único estable entre las dos
 * aplicaciones. Sytue Notepads guarda exactamente ese valor como destino.
 *
 * Es el otro extremo de la referencia bidireccional con Sytue Notepads: allí una
 * nota puede colgar de un evento, y desde aquí se pregunta quién cuelga de este.
 *
 * EL DATO VIVE UNA SOLA VEZ, en Sytue Notepads (tabla `note_links`). Calendar no
 * guarda nada: pregunta con el token de QUIEN MIRA, así que la autorización
 * sigue siendo la de Notes —una nota privada no aparece en el evento de otra
 * persona— y no hay dos copias del vínculo que se desincronicen al borrar una.
 *
 * Si Sytue Notepads no está desplegado, se responde con una lista vacía en vez de un
 * error: la ficha del evento no tiene por qué romperse porque falte una app
 * opcional de la suite.
 */
import type { APIRoute } from 'astro';
import { NotesClient, bearer } from '@sytue/api-clients';
import { rutaConOrg } from '@sytue/config';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { json } from '../../lib/caldav.js';

export const prerender = false;

const env = serverEnv(import.meta.env);

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const eventUid = new URL(ctx.request.url).searchParams.get('event')?.trim();
  if (!eventUid) return json({ error: 'missing_event' }, 400);

  const base = env.SYTUE_NOTES_API_URL ?? env.PUBLIC_SYTUE_NOTES_BASE_URL;
  if (!base) return json({ notes: [], notesUrl: null });

  const client = new NotesClient({ baseUrl: base, auth: bearer(session.accessToken) });
  try {
    const notes = await client.linkedNotes('calendar_event', eventUid, 20);
    return json({
      notes: notes.map((n) => ({
        id: n.id,
        notebook_id: n.notebook_id,
        title: n.title ?? '',
        encrypted: Boolean(n.content_cipher),
      })),
      notesUrl: env.PUBLIC_SYTUE_NOTES_BASE_URL
        ? new URL(rutaConOrg('/notes', ctx.locals.org, '/'), env.PUBLIC_SYTUE_NOTES_BASE_URL).href
        : null,
    });
  } catch {
    // Degradación elegante: sin Notes, el evento sigue abriéndose igual.
    return json({ notes: [], notesUrl: null });
  }
};
