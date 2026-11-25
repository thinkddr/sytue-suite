/**
 * Retirar una passkey, o cambiarle el nombre.
 *
 *   DELETE /api/passkeys/{id}
 *   PATCH  /api/passkeys/{id}   { name }
 *
 * Retirar: el servicio rechaza la última si es el único factor con el que cumplir
 * la verificación en dos pasos —dejaría la cuenta exigiendo algo imposible—, y ese
 * mensaje se enseña tal cual.
 *
 * Cambiar el nombre: solo se reenvía ese campo, como en el resto de rutas de esta
 * app; el tope y el saneado los aplica Sytue ID.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

const ruta = (id: string): string => `/api/v1/webauthn/credentials/${encodeURIComponent(id)}`;

export const DELETE: APIRoute = async (ctx) => {
  const id = ctx.params.id ?? '';
  if (!id) return new Response(null, { status: 400 });
  return proxyId(ctx, ruta(id), { method: 'DELETE' });
};

export const PATCH: APIRoute = async (ctx) => {
  const id = ctx.params.id ?? '';
  let body: { name?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return new Response(null, { status: 400 });
  }
  const name = typeof body.name === 'string' ? body.name.trim() : '';
  if (!id || !name) return new Response(null, { status: 400 });
  return proxyId(ctx, ruta(id), {
    method: 'PATCH',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ name }),
  });
};
