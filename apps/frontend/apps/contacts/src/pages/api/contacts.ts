/**
 * GET /api/contacts?addressBookUrl= — lista los contactos de una libreta
 * (addressbook-query REPORT via el SDK CardDAV). Proxy BFF autorizado por cookie.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { cardDavClient, rutaDav } from '../../lib/carddav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const addressBookUrl = rutaDav(ctx.url.searchParams.get('addressBookUrl'));
  if (!addressBookUrl) return json({ error: 'missing_addressBookUrl' }, 400);

  try {
    const contacts = await cardDavClient(session.accessToken, session.user.id).listContacts(
      addressBookUrl,
    );
    return json({ contacts });
  } catch (err) {
    return json({ error: 'carddav_error', message: 'CardDAV request failed' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
