/**
 * Olvidar un dispositivo. Puede ser cualquiera de los míos, también uno perdido
 * en el que ya no estoy: deja de reconocerse y pierde la confianza.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const DELETE: APIRoute = async (ctx) => {
  const id = ctx.params.id ?? '';
  if (!id) return new Response(null, { status: 400 });
  return proxyId(ctx, `/api/v1/me/devices/${encodeURIComponent(id)}`, { method: 'DELETE' });
};
