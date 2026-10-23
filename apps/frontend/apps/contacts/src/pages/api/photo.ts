/**
 * Foto de contacto (avatar) — proxy BFF autorizado por cookie hacia la API REST
 * del servicio Go Sytue Contacts. La foto vive EMBEBIDA en la PHOTO de la vCard
 * (data URI); aquí se sirve/define/quita, siempre owner-scoped en el servicio Go.
 *
 * GET  /api/photo?uid=&v=        -> GET    /api/v1/contacts/photo?uid=  (bytes, <img src>)
 * POST /api/photo { uid, data }  -> PUT    /api/v1/contacts/photo       (definir)
 * POST /api/photo { uid, remove }-> DELETE /api/v1/contacts/photo?uid=  (quitar)
 *
 * Las mutaciones van por POST con cuerpo JSON (CSRF-safe). El GET devuelve bytes
 * de imagen (no data URI) para no inflar el DOM y aprovechar la caché del
 * navegador (etag + ?v=<etag>). El access token nunca llega al navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { restJson, RestError } from '../../lib/rest.js';
import { CONTACTS_BASE_URL } from '../../lib/carddav.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response(null, { status: 401 });

  const uid = ctx.url.searchParams.get('uid');
  if (!uid) return new Response(null, { status: 400 });

  // Reenvía la revalidación condicional del navegador para poder relayar 304.
  const inm = ctx.request.headers.get('if-none-match');
  const upstream = await fetch(
    `${CONTACTS_BASE_URL}/api/v1/contacts/photo?uid=${encodeURIComponent(uid)}`,
    {
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        accept: 'image/*',
        ...(inm ? { 'if-none-match': inm } : {}),
      },
    },
  ).catch(() => null);

  if (!upstream) return new Response(null, { status: 502 });
  if (upstream.status === 304) {
    return new Response(null, { status: 304, headers: passthroughCache(upstream) });
  }
  if (!upstream.ok) return new Response(null, { status: upstream.status });

  return new Response(upstream.body, {
    status: 200,
    headers: {
      'content-type': upstream.headers.get('content-type') ?? 'application/octet-stream',
      'x-content-type-options': 'nosniff',
      ...passthroughCache(upstream),
    },
  });
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as {
    uid?: string;
    data?: string;
    remove?: boolean;
  };
  const uid = body.uid?.trim();
  if (!uid) return json({ error: 'missing_uid' }, 400);

  try {
    if (body.remove === true) {
      await restJson<void>(
        session.accessToken,
        'DELETE',
        `/api/v1/contacts/photo?uid=${encodeURIComponent(uid)}`,
      );
      return json({ ok: true });
    }
    if (!body.data) return json({ error: 'missing_data' }, 400);
    const res = await restJson(session.accessToken, 'PUT', '/api/v1/contacts/photo', {
      uid,
      data: body.data,
    });
    return json(res);
  } catch (err) {
    if (err instanceof RestError)
      return json(
        { error: 'contacts_error', message: 'Contacts service request failed' },
        err.status,
      );
    return json({ error: 'contacts_error', message: 'Contacts service request failed' }, 502);
  }
};

function passthroughCache(res: Response): Record<string, string> {
  const headers: Record<string, string> = { 'cache-control': 'private, max-age=3600' };
  const etag = res.headers.get('etag');
  if (etag) headers.etag = etag;
  return headers;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
