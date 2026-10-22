/**
 * Acciones EN LOTE (multiselección) — proxy BFF autorizado por cookie hacia la API
 * REST del servicio Go Sytue Contacts.
 *
 * POST /api/batch { action, uids, ... }
 *   action = 'trash'    -> POST /api/v1/contacts/batch/trash      { uids }
 *   action = 'favorite' -> POST /api/v1/contacts/batch/favorite   { uids, favorite }
 *   action = 'group'    -> POST /api/v1/groups/{groupId}/members  { uids, add }
 *
 * Mutaciones por POST con cuerpo JSON (CSRF-safe: el navegador no puede enviarlas
 * cross-origin sin preflight; Astro además rechaza DELETE/PUT). El servicio Go
 * resuelve cada acción en UNA consulta `uid = ANY` y SOLO sobre contactos del
 * propio dueño. El access token nunca llega al navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { restJson, RestError } from '../../lib/rest.js';

export const prerender = false;

interface BatchBody {
  action?: string;
  uids?: unknown;
  favorite?: boolean;
  groupId?: string;
  add?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as BatchBody;
  const uids = Array.isArray(body.uids)
    ? body.uids.filter((u): u is string => typeof u === 'string')
    : [];
  if (uids.length === 0) return json({ error: 'missing_uids' }, 400);

  try {
    switch (body.action) {
      case 'trash':
        return json(
          await restJson(session.accessToken, 'POST', '/api/v1/contacts/batch/trash', { uids }),
        );
      case 'favorite':
        return json(
          await restJson(session.accessToken, 'POST', '/api/v1/contacts/batch/favorite', {
            uids,
            favorite: body.favorite === true,
          }),
        );
      case 'group': {
        const groupId = body.groupId?.trim();
        if (!groupId) return json({ error: 'missing_group' }, 400);
        return json(
          await restJson(
            session.accessToken,
            'POST',
            `/api/v1/groups/${encodeURIComponent(groupId)}/members`,
            { uids, add: body.add === true },
          ),
        );
      }
      default:
        return json({ error: 'invalid_action' }, 400);
    }
  } catch (err) {
    return fail(err);
  }
};

function fail(err: unknown): Response {
  if (err instanceof RestError)
    return json(
      { error: 'contacts_error', message: 'Contacts service request failed' },
      err.status,
    );
  return json({ error: 'contacts_error', message: 'Contacts service request failed' }, 502);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
