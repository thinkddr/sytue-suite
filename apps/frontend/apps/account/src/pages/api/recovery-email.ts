/**
 * Correo de recuperación (ADR 0041).
 *
 *   PUT /api/recovery-email   { email, password }
 *
 * Con `email`, Sytue ID manda un enlace de confirmación a esa dirección (202) y
 * no la fija hasta que se abre. Con `email` vacío, retira la actual (200). Las
 * dos piden la contraseña: con una sesión robada, poner un correo propio como
 * recuperación y pedir después el reseteo sería quedarse la cuenta.
 *
 * Solo se reenvían esos dos campos, como en el resto de rutas de esta app.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const PUT: APIRoute = async (ctx) => {
  let body: { email?: unknown; password?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return new Response(null, { status: 400 });
  }
  const email = typeof body.email === 'string' ? body.email.trim() : '';
  const password = typeof body.password === 'string' ? body.password : '';
  if (!password) return new Response(null, { status: 400 });

  return proxyId(ctx, '/api/v1/me/recovery-email', {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
};
