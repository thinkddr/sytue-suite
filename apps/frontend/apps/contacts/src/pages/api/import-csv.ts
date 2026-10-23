/**
 * Importación CSV (Google Contacts / Outlook) — proxy BFF autorizado por cookie.
 *
 * POST /api/import-csv { addressBookUrl, csv } -> POST /api/v1/address-books/{uuid}/import-csv
 *
 * El navegador envía el texto CSV (leído del fichero) por POST JSON; el BFF lo
 * reenvía como cuerpo text/csv crudo al servicio Go, que detecta el dialecto,
 * salta filas inválidas, deduplica por correo y responde { imported, skipped }.
 * El servicio Go acota el tamaño con MaxBytesReader; aquí se rechaza pronto un
 * CSV desproporcionado (defensa en profundidad). El token nunca llega al cliente.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { resolveAddressBookId } from '../../lib/rest.js';
import { CONTACTS_BASE_URL } from '../../lib/carddav.js';

export const prerender = false;

// Límite de tamaño del CSV (5 MiB), alineado con el MaxBytesReader del servicio Go.
const MAX_CSV_BYTES = 5 << 20;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as {
    addressBookUrl?: string;
    csv?: string;
  };
  if (!body.addressBookUrl || typeof body.csv !== 'string') {
    return json({ error: 'invalid_payload' }, 400);
  }
  if (body.csv.length > MAX_CSV_BYTES) {
    return json({ error: 'csv_too_large' }, 413);
  }

  try {
    const bookId = await resolveAddressBookId(session.accessToken, body.addressBookUrl);
    if (!bookId) return json({ error: 'addressbook_not_found' }, 404);

    const res = await fetch(
      `${CONTACTS_BASE_URL}/api/v1/address-books/${encodeURIComponent(bookId)}/import-csv`,
      {
        method: 'POST',
        headers: {
          authorization: `Bearer ${session.accessToken}`,
          'content-type': 'text/csv; charset=utf-8',
          accept: 'application/json',
        },
        body: body.csv,
      },
    );
    if (!res.ok) {
      return json(
        { error: 'import_failed', message: 'Contacts import failed' },
        res.status >= 500 ? 502 : res.status,
      );
    }
    // { imported, skipped }
    return json(await res.json());
  } catch (err) {
    return json({ error: 'contacts_error', message: 'Contacts service request failed' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
