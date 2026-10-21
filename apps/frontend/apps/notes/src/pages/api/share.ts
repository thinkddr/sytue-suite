/**
 * POST /api/share — publica una nota en una sala de Sytue Rooms.
 *
 * Lo que se manda es un ENLACE a la nota, no su contenido: la sala puede tener
 * gente que no debería leerla, y quien la abra pasará por la autorización de
 * Notes como cualquier otro día. Rooms ya despliega la vista previa de los
 * enlaces de la suite, así que el mensaje se ve como una tarjeta sin que este
 * endpoint tenga que fabricarla.
 *
 * El texto lo compone el SERVIDOR a partir del título que trae el cliente: así
 * el cuerpo del mensaje no es texto arbitrario de una petición.
 */
import type { APIRoute } from 'astro';
import { ChatClient, bearer } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { cuerpo, fallo, json } from '../../lib/notes.js';
import { mensajeNota } from '../../lib/elegibles.js';

export const prerender = false;

const env = serverEnv(import.meta.env);

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const b = await cuerpo<{
    channel_id?: string;
    notebook_id?: string;
    note_id?: string;
    title?: string;
  }>(ctx);
  if (!b.channel_id || !b.notebook_id || !b.note_id) {
    return json({ error: 'missing_params' }, 400);
  }

  const client = new ChatClient({
    baseUrl: env.SYTUE_ROOMS_API_URL ?? 'https://rooms.sytue.eu',
    auth: bearer(session.accessToken),
  });
  try {
    const msg = await client.send({
      channelId: b.channel_id,
      body: mensajeNota(b.title ?? '', b.notebook_id, b.note_id, session.user.tenant),
    });
    return json({ id: msg.id });
  } catch (err) {
    return fallo(err);
  }
};
