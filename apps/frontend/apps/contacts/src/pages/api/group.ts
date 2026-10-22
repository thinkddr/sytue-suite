/**
 * Mutar un grupo (etiqueta) por ID — proxy BFF autorizado por cookie hacia la
 * API REST del servicio Go Sytue Contacts. Todo por POST: el CSRF de Astro
 * rechaza DELETE/PUT con cuerpo desde el navegador.
 *
 * POST /api/group { id, delete: true }      -> DELETE /api/v1/groups/{id}
 * POST /api/group { id, name?, members }    -> PUT    /api/v1/groups/{id}
 *                                              (PUT reemplaza la pertenencia
 *                                              completa: members obligatorio)
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { restJson, RestError } from '../../lib/rest.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as {
    id?: string;
    delete?: boolean;
    name?: string;
    members?: string[];
  };
  if (!body.id) return json({ error: 'missing_id' }, 400);
  const path = `/api/v1/groups/${encodeURIComponent(body.id)}`;

  try {
    if (body.delete === true) {
      await restJson<void>(session.accessToken, 'DELETE', path);
      return json({ ok: true });
    }
    // El PUT del servicio Go reemplaza la lista de miembros entera: exigir el
    // array evita que un rename despistado vacíe la etiqueta.
    if (!Array.isArray(body.members)) return json({ error: 'missing_members' }, 400);
    const group = await restJson<unknown>(session.accessToken, 'PUT', path, {
      name: body.name ?? '',
      members: body.members,
    });
    return json(group);
  } catch (err) {
    if (err instanceof RestError)
      return json(
        { error: 'contacts_error', message: 'Contacts service request failed' },
        err.status,
      );
    return json({ error: 'contacts_error', message: 'Contacts service request failed' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
