/**
 * GET /api/backlinks?note=<id> — QUÉ NOTAS APUNTAN A ESTA.
 *
 * Es la vuelta de la referencia. Sytue Notepads ya dejaba enlazar una nota con
 * otra —`crossref`, y con cuadernos, documentos, ficheros, eventos y canales—,
 * pero solo se veía el enlace desde el lado que lo creó: al abrir la nota
 * apuntada no había forma de saber quién la señalaba. Un enlace que solo se ve
 * desde un extremo es una nota suelta con una flecha, no una red.
 *
 * Es lo que BasKet Note Pads hacía con las cestas y lo que aquí se puede llevar
 * más lejos, porque el mismo mecanismo vale para cualquier objeto de la suite:
 * las notas que cuelgan de un evento, de un documento o de un canal salen de
 * este mismo sitio cambiando el tipo.
 *
 * EL DATO VIVE UNA SOLA VEZ, en `note_links`, y se pregunta con el token de
 * QUIEN MIRA: una nota privada de otra persona no aparece como retroenlace, sin
 * que esta capa tenga que filtrar nada.
 */
import type { APIRoute } from 'astro';
import type { NoteLinkTarget } from '@sytue/api-clients';
import { conCliente, fallo, json } from '../../lib/notes.js';

export const prerender = false;

// Los destinos que el servicio reconoce. Se valida aquí en vez de reenviar lo
// que llegue: un `target_kind` inventado no da un error claro abajo —devuelve
// una lista vacía, que se lee como «esta nota no tiene retroenlaces»— y ese es
// el peor resultado posible, porque parece una respuesta.
const DESTINOS: readonly NoteLinkTarget[] = [
  'calendar_event',
  'room_channel',
  'room_message',
  'docs_document',
  'drive_file',
  'notebook',
  'note',
];

function esDestino(v: string): v is NoteLinkTarget {
  return (DESTINOS as readonly string[]).includes(v);
}

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;

  const url = new URL(ctx.request.url);
  // `note` es el caso corriente y se escribe corto; `target_kind`/`target_id`
  // dejan preguntar por cualquier otro objeto sin un endpoint por tipo.
  const id = (url.searchParams.get('note') ?? url.searchParams.get('target_id') ?? '').trim();
  const kind = (url.searchParams.get('target_kind') ?? 'note').trim();
  if (!id) return json({ error: 'missing_target' }, 400);
  if (!esDestino(kind)) return json({ error: 'invalid_target_kind' }, 400);

  try {
    return json({ notes: await c.client.linkedNotes(kind, id, 50) });
  } catch (err) {
    return fallo(err);
  }
};
