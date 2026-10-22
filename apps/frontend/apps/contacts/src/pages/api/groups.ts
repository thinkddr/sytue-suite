/**
 * Grupos (etiquetas) de una libreta — proxy BFF autorizado por cookie hacia la
 * API REST del servicio Go Sytue Contacts.
 *
 * GET  /api/groups?addressBookUrl= -> GET  /api/v1/address-books/{uuid}/groups
 * POST /api/groups                 -> POST /api/v1/address-books/{uuid}/groups
 *                                     cuerpo: { addressBookUrl, name, members? }
 *
 * La API Go direcciona por UUID de libreta; el frontend solo tiene la URL
 * CardDAV (slug), así que aquí se resuelve slug -> UUID (lib/rest.ts).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { resolveAddressBookId, restJson, RestError } from '../../lib/rest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const addressBookUrl = ctx.url.searchParams.get('addressBookUrl');
  if (!addressBookUrl) return json({ error: 'missing_addressBookUrl' }, 400);

  try {
    const bookId = await resolveAddressBookId(session.accessToken, addressBookUrl);
    if (!bookId) return json({ error: 'addressbook_not_found' }, 404);
    const body = await restJson<{ groups: unknown[] }>(
      session.accessToken,
      'GET',
      `/api/v1/address-books/${encodeURIComponent(bookId)}/groups`,
    );
    return json(body);
  } catch (err) {
    return fail(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as {
    addressBookUrl?: string;
    name?: string;
    members?: string[];
  };
  if (!body.addressBookUrl || !body.name?.trim()) return json({ error: 'invalid_payload' }, 400);

  try {
    const bookId = await resolveAddressBookId(session.accessToken, body.addressBookUrl);
    if (!bookId) return json({ error: 'addressbook_not_found' }, 404);
    const group = await restJson<unknown>(
      session.accessToken,
      'POST',
      `/api/v1/address-books/${encodeURIComponent(bookId)}/groups`,
      { name: body.name.trim(), members: body.members ?? [] },
    );
    return json(group, 201);
  } catch (err) {
    return fail(err);
  }
};

function fail(err: unknown): Response {
  if (err instanceof RestError)
    return json({ error: 'contacts_error', message: err.message }, err.status);
  return json({ error: 'contacts_error', message: String(err) }, 502);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
