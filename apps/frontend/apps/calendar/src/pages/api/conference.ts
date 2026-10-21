/**
 * POST /api/conference — crea (o reutiliza) la videollamada de un evento.
 *
 * Cuerpo: { uid: string, title: string } → { url: string }
 *
 * La sala vive en Sytue Rooms, no en Calendar: aquí solo se pide, ligada al evento por
 * su UID (`kind=calendar`, `ref_id=<uid>`). `from-source` es IDEMPOTENTE por origen, así
 * que reabrir el evento y volver a pulsar devuelve LA MISMA sala en vez de una nueva —
 * que es justo lo que hace falta para que el enlace del invitado siga sirviendo.
 *
 * La URL que se devuelve es la pública de Rooms con `?rooms=<id>` (`enlaceReunion`): la misma que copia
 * «Copiar el enlace» del panel de reuniones, para que un invitado sin canal pueda entrar.
 * Se guarda en la propiedad CONFERENCE del evento (RFC 7986), que es la que hace que
 * Apple Calendar y Outlook pinten «Unirse» al sincronizar por CalDAV.
 *
 * Proxy BFF puro: el access token de Sytue ID no llega al navegador y la autorización la
 * decide el servicio Go.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { enlaceReunion } from '@sytue/config';
import { getApiSession } from '../../lib/session.js';
import { fallo, json } from '../../lib/caldav.js';

export const prerender = false;

/** API interna de Rooms (server-to-server). */
function roomsApi(env: ImportMetaEnv): string {
  return (serverEnv(env).SYTUE_ROOMS_API_URL ?? 'https://rooms.sytue.eu').replace(/\/$/, '');
}

/** Host PÚBLICO de Rooms: el que se pone en el enlace que abre el invitado. */
function roomsPublic(env: ImportMetaEnv): string {
  return (serverEnv(env).SYTUE_ROOMS_PUBLIC_URL ?? 'https://rooms.sytue.eu').replace(/\/$/, '');
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { uid?: string; title?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const uid = payload.uid?.trim();
  if (!uid) return json({ error: 'missing_uid' }, 400);
  const title = payload.title?.trim() || 'Reunión';

  try {
    const res = await fetch(`${roomsApi(import.meta.env)}/api/v1/rooms/from-source`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify({ kind: 'calendar', ref_id: uid, title }),
    });
    if (!res.ok)
      return json(
        { error: 'rooms_error', status: res.status },
        res.status < 500 ? res.status : 502,
      );
    const room = (await res.json()) as { id?: string; link_key?: string };
    if (!room.id) return json({ error: 'rooms_error' }, 502);
    // Con la clave: la reunión de un evento es suelta, y los invitados entran por el
    // enlace que les llega en el evento.
    return json({
      url: enlaceReunion(room.id, room.link_key ?? '', roomsPublic(import.meta.env)),
    });
  } catch (err) {
    return fallo(err, 'rooms_error');
  }
};
