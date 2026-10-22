/**
 * Apaga el segundo factor. El servicio exige prueba de identidad —un código
 * vigente O la contraseña—, que es lo que impide que una sesión secuestrada lo
 * apague sin más. Va por POST y no por DELETE porque lleva cuerpo.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  let body: { code?: unknown; password?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return new Response(null, { status: 400 });
  }
  const code = typeof body.code === 'string' ? body.code.trim() : '';
  const password = typeof body.password === 'string' ? body.password : '';
  if (!code && !password) return new Response(null, { status: 400 });
  return proxyId(ctx, '/api/v1/me/mfa/totp', {
    method: 'DELETE',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ code, password }),
  });
};
