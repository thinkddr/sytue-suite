/**
 * Cambiar o añadir el móvil (ADR 0041).
 *
 *   PUT  /api/phone   { phone, password }   manda el código por SMS al móvil nuevo (202)
 *   POST /api/phone   { code }              lo confirma y lo fija (200)
 *
 * El móvil no se fija hasta confirmar el código, y que ya es de otra cuenta solo lo
 * dice Sytue ID después del código. Es también como una cuenta sin móvil añade uno.
 *
 * Solo se reenvían esos campos, como en el resto de rutas de esta app.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

async function cuerpo(ctx: Parameters<APIRoute>[0]): Promise<Record<string, unknown> | null> {
  try {
    return (await ctx.request.json()) as Record<string, unknown>;
  } catch {
    return null;
  }
}

export const PUT: APIRoute = async (ctx) => {
  const body = await cuerpo(ctx);
  const phone = typeof body?.phone === 'string' ? body.phone.trim() : '';
  const password = typeof body?.password === 'string' ? body.password : '';
  if (!phone || !password) return new Response(null, { status: 400 });
  return proxyId(ctx, '/api/v1/me/phone', {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ phone, password }),
  });
};

export const POST: APIRoute = async (ctx) => {
  const body = await cuerpo(ctx);
  const code = typeof body?.code === 'string' ? body.code.trim() : '';
  if (!/^\d{6}$/.test(code)) return new Response(null, { status: 400 });
  return proxyId(ctx, '/api/v1/me/phone/verify', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ code }),
  });
};
