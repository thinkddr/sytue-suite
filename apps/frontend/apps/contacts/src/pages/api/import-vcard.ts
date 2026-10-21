/**
 * Importar UNA tarjeta vCard de fuera — proxy BFF autorizado por cookie.
 *
 * POST /api/import-vcard { addressBookId, vcard } -> POST /api/v1/address-books/{id}/contacts
 *                                                    (Content-Type: text/vcard)
 *
 * La tarjeta va ENTERA al servicio, que es quien la entiende (`domain.ImportCard`):
 * conserva los X-* de otros clientes y pasa la foto binaria de vCard 3.0
 * (`PHOTO;ENCODING=b`) a data URI, que es lo que se pinta. Antes se reconstruía
 * aquí con `buildVcard` a partir de lo que el navegador sabía leer, y esa foto se
 * guardaba tal cual: estaba en la tarjeta, pero no se veía. Es la misma alta que
 * usa la app nativa. El token nunca llega al cliente.
 */
import type { APIRoute } from 'astro';
import { HttpError } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { contactsRestClient } from '../../lib/carddav.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as {
    addressBookId?: unknown;
    vcard?: unknown;
  };
  if (typeof body.addressBookId !== 'string' || !body.addressBookId) {
    return json({ error: 'invalid_payload' }, 400);
  }
  if (typeof body.vcard !== 'string' || !/^\s*BEGIN:VCARD/i.test(body.vcard)) {
    return json({ error: 'invalid_payload' }, 400);
  }

  try {
    const c = await contactsRestClient(session.accessToken).importCard(
      body.addressBookId,
      body.vcard,
    );
    return json({ id: c.id, uid: c.uid }, 201);
  } catch (err) {
    // 4xx del servicio (tarjeta inválida, 413 por tamaño, libreta ajena) tal cual.
    const status = err instanceof HttpError && err.status < 500 ? err.status : 502;
    return json({ error: 'import_failed' }, status);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
