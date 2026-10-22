/**
 * POST /api/purge — borra correo DE VERDAD y para siempre ({ ids: […] }).
 *
 * Hasta ahora la papelera era un sitio del que no se salía: la app sabía mover
 * mensajes a ella, pero el borrado definitivo del servicio (DELETE
 * /api/v1/messages/{id}) no tenía ninguna puerta. Y ese borrado es lo ÚNICO que
 * devuelve espacio —descuenta `used_bytes` de la cuenta—, así que el medidor de
 * almacenamiento del lateral podía marcar el buzón lleno sin ofrecer ningún
 * remedio. No hay purgado automático de la papelera en el servicio.
 *
 * Va en su propia ruta y no en /api/message-actions a propósito: esto es
 * irreversible y conviene que se vea de lejos, en el código y en los registros.
 * Proxy BFF autorizado por cookie; el aislamiento por dueño lo impone el
 * servicio (account del token + RLS), no esta puerta.
 *
 * POST y no DELETE porque el checkOrigin CSRF de Astro rechaza DELETE con cuerpo
 * desde el navegador (mismo trato que /api/batch y /api/labels).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailDelete } from '../../lib/mailrest.js';

export const prerender = false;

/**
 * Tope de mensajes por petición.
 *
 * ponytail: el servicio no tiene borrado en lote (solo /messages/batch/move y
 * /batch/flags), así que esto son N llamadas secuenciales. Basta para lo que se
 * borra a mano —una conversación, una selección—; si algún día hace falta un
 * «vaciar la papelera» de miles, el sitio donde arreglarlo es el servicio con un
 * endpoint set-based, no un bucle más largo aquí.
 */
const MAX_IDS = 100;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { ids?: unknown };
  const ids = (Array.isArray(body.ids) ? body.ids : []).filter(
    (v): v is string => typeof v === 'string' && v !== '',
  );
  if (ids.length === 0 || ids.length > MAX_IDS) return json({ error: 'invalid_payload' }, 400);

  let deleted = 0;
  try {
    for (const id of ids) {
      const res = await mailDelete(
        session.accessToken,
        `/api/v1/messages/${encodeURIComponent(id)}`,
      );
      // El 404 se cuenta como hecho: el mensaje ya no está, que es lo que se pedía.
      if (res.ok || res.status === 404) deleted += 1;
      else if (res.status === 401 || res.status === 403) return json({ error: 'forbidden' }, 403);
    }
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
  // Un borrado parcial es un fallo para quien lo pidió: si no cayeron todos, se
  // dice, porque la lista de la pantalla ya los quitó todos de la vista.
  return json({ deleted }, deleted === ids.length ? 200 : 502);
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
