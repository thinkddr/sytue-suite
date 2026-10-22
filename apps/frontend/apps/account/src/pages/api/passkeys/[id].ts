/**
 * Retirar una passkey. El servicio rechaza la última si es el único factor con
 * el que cumplir la verificación en dos pasos —dejaría la cuenta exigiendo algo
 * imposible—, y ese mensaje se enseña tal cual.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const DELETE: APIRoute = async (ctx) => {
  const id = ctx.params.id ?? '';
  if (!id) return new Response(null, { status: 400 });
  return proxyId(ctx, `/api/v1/webauthn/credentials/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
};
