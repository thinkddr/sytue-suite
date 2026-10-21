/**
 * Avatar propio (predefinido o foto) — proxy BFF hacia Sytue ID.
 *
 *   PUT    /api/avatar    -> PUT    /api/v1/me/avatar   {photo: data URI}
 *   DELETE /api/avatar    -> DELETE /api/v1/me/avatar
 *
 * El GET ya no está aquí: la foto la sirve `/auth/avatar`, el MISMO handler que
 * usa la barra de las trece apps (`@sytue/auth`, `bff.avatar`). Eran dos copias
 * del mismo proxy, y la de la barra era la que faltaba.
 */
import type { APIRoute } from 'astro';
import { getApiSession, proxyId } from '../../lib/bff.js';

export const prerender = false;

export const PUT: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response(null, { status: 401 });
  const body = await ctx.request.text();
  const upstream = await proxyId(ctx, '/api/v1/me/avatar', {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body,
  });
  return new Response(upstream.body, { status: upstream.status });
};

export const DELETE: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response(null, { status: 401 });
  const upstream = await proxyId(ctx, '/api/v1/me/avatar', { method: 'DELETE' });
  return new Response(upstream.body, { status: upstream.status });
};
