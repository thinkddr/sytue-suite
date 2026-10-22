/**
 * /api/aliases — CRUD de alias de correo (direcciones adicionales que entregan a
 * la misma cuenta), proxy BFF al servicio Go (REST /api/v1/aliases). GET lista +
 * la primaria; POST multiplexa por la forma del cuerpo, como el resto de la app
 * (el checkOrigin CSRF de Astro rechaza PUT/DELETE desde el navegador):
 *   { address }            → crear
 *   { id, delete:true }    → borrar
 * Autorizado por cookie de sesión; la autorización por dueño y la validación de
 * dominio/unicidad las impone el servicio.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost, mailOrigin } from '../../lib/mailrest.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;

interface RawAlias {
  id: string;
  address?: string;
}

function mapAlias(a: RawAlias) {
  return { id: a.id, address: a.address ?? '' };
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/aliases');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { aliases?: RawAlias[]; primary?: string };
    return json({ aliases: (body.aliases ?? []).map(mapAlias), primary: body.primary ?? '' });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

interface Payload {
  id?: string;
  address?: string;
  delete?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  const token = session.accessToken;

  try {
    // Borrar.
    if (body.id && body.delete === true) {
      if (!UUID.test(body.id)) return json({ error: 'invalid_id' }, 400);
      const res = await fetch(`${mailOrigin()}/api/v1/aliases/${body.id}`, {
        method: 'DELETE',
        headers: { authorization: `Bearer ${token}`, accept: 'application/json' },
      });
      if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
      return json({ ok: true });
    }
    // Crear (address sin id).
    if (typeof body.address === 'string' && body.address.trim()) {
      const res = await mailPost(token, '/api/v1/aliases', { address: body.address.trim() });
      if (!res.ok) {
        // 400/409 traen un código estable del servicio (alias_reserved, alias_limit…)
        // que el modal traduce; el resto es fallo nuestro y se colapsa a 502.
        const cuerpo = (await res.json().catch(() => ({}))) as { code?: string };
        const propio = res.status === 400 || res.status === 409;
        return json(
          { error: 'mail_error', code: cuerpo.code, status: res.status },
          propio ? res.status : 502,
        );
      }
      return json({ alias: mapAlias((await res.json()) as RawAlias) }, 201);
    }
    return json({ error: 'invalid_payload' }, 400);
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
