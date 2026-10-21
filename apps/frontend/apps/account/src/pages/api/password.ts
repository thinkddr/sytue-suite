/**
 * Cambio de contraseña.
 *
 * El servicio REVOCA TODAS las sesiones al cambiarla —también la de este BFF—,
 * así que la respuesta se acompaña de un barrido de la cookie de sesión: sin él
 * quedaría una sesión de navegador viva cuyo refresh ya no vale, y la pantalla
 * se iría degradando a 401 sin decir por qué.
 */
import type { APIRoute } from 'astro';
import { SESSION_COOKIE } from '@sytue/auth/server';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  let body: { current_password?: unknown; new_password?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return new Response(null, { status: 400 });
  }
  const actual = typeof body.current_password === 'string' ? body.current_password : '';
  const nueva = typeof body.new_password === 'string' ? body.new_password : '';
  if (!actual || !nueva) return new Response(null, { status: 400 });

  const res = await proxyId(ctx, '/api/v1/me/password', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ current_password: actual, new_password: nueva }),
  });
  if (res.ok) {
    for (const nombre of [SESSION_COOKIE, `__Host-${SESSION_COOKIE}`]) {
      ctx.cookies.delete(nombre, { path: '/' });
    }
  }
  return res;
};
