/**
 * /api/addressbooks — libretas de direcciones del usuario.
 *
 *   GET    /api/addressbooks               Lista las libretas (CardDAV).
 *   POST   /api/addressbooks     {displayName}       Crea una libreta.
 *   PATCH  /api/addressbooks     {id, displayName}   Renombra una libreta.
 *   DELETE /api/addressbooks?id=…                     Borra una libreta.
 *
 * El listado va por CardDAV (como siempre); crear/renombrar/borrar van por la
 * API REST propia del servicio (`/api/v1/address-books`), que no tiene
 * equivalente en los verbos WebDAV que este BFF hablaba hasta ahora. Proxy
 * autorizado por cookie: el access token nunca llega al navegador.
 */
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { cardDavClient, contactsRestClient } from '../../lib/carddav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const addressBooks = await cardDavClient(
      session.accessToken,
      session.user.id,
    ).listAddressBooks();
    // El id REST (para renombrar/borrar) no viaja por CardDAV: se empareja aquí
    // por el segmento final de la URL, que ES el `name` (slug) de la libreta en
    // los dos protocolos — mismo dato, dos superficies. Un fallo aquí no rompe
    // el listado: la libreta sale igual, solo sin id (sin renombrar/borrar).
    let porNombre = new Map<string, { id: string; isDefault?: boolean }>();
    try {
      const rest = await contactsRestClient(session.accessToken).listAddressBooks();
      porNombre = new Map(
        rest
          .filter((b) => b.name)
          .map((b) => [b.name, { id: b.id, ...(b.isDefault ? { isDefault: true } : {}) }]),
      );
    } catch {
      // sigue con el listado CardDAV a secas
    }
    const conId = addressBooks.map((b) => {
      const slug = b.url.replace(/\/+$/, '').split('/').pop() ?? '';
      const extra = porNombre.get(slug);
      return extra ? { ...b, ...extra } : b;
    });
    return json({ addressBooks: conId });
  } catch {
    return json({ error: 'carddav_error', message: 'CardDAV request failed' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => null)) as { displayName?: unknown } | null;
  const displayName = typeof body?.displayName === 'string' ? body.displayName.trim() : '';
  if (!displayName) return json({ error: 'missing_display_name' }, 400);
  try {
    const addressBook = await contactsRestClient(session.accessToken).createAddressBook(
      displayName,
    );
    return json({ addressBook }, 201);
  } catch (err) {
    return json(
      { error: 'contacts_error', message: 'Contacts service request failed' },
      upstreamStatus(err),
    );
  }
};

export const PATCH: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => null)) as {
    id?: unknown;
    displayName?: unknown;
  } | null;
  const id = typeof body?.id === 'string' ? body.id.trim() : '';
  const displayName = typeof body?.displayName === 'string' ? body.displayName.trim() : '';
  if (!id || !displayName) return json({ error: 'missing_fields' }, 400);
  try {
    const addressBook = await contactsRestClient(session.accessToken).renameAddressBook(
      id,
      displayName,
    );
    return json({ addressBook });
  } catch (err) {
    return json(
      { error: 'contacts_error', message: 'Contacts service request failed' },
      upstreamStatus(err),
    );
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const id = new URL(ctx.request.url).searchParams.get('id');
  if (!id) return json({ error: 'missing_id' }, 400);
  try {
    await contactsRestClient(session.accessToken).deleteAddressBook(id);
    return json({ ok: true });
  } catch (err) {
    return json(
      { error: 'contacts_error', message: 'Contacts service request failed' },
      upstreamStatus(err),
    );
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
