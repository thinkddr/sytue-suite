/** Revocar una contraseña de aplicación. El cliente que la usara deja de entrar. */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const DELETE: APIRoute = async (ctx) => {
  const id = ctx.params.id ?? '';
  if (!id) return new Response(null, { status: 400 });
  return proxyId(ctx, `/api/v1/me/app-passwords/${encodeURIComponent(id)}`, { method: 'DELETE' });
};
