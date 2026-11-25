/**
 * Cambiar el @usuario (ADR 0040).
 *
 *   PUT /api/username   { username, password }
 *
 * Pide la contraseña: el @usuario es un identificador de acceso. El anterior queda
 * reservado 30 días para la misma persona, y no se cambia dos veces en ese plazo; lo
 * decide Sytue ID, que contesta con un código por cada negativa.
 *
 * Solo se reenvían esos dos campos, como en el resto de rutas de esta app.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const PUT: APIRoute = async (ctx) => {
  let body: { username?: unknown; password?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return new Response(null, { status: 400 });
  }
  const username = typeof body.username === 'string' ? body.username.trim() : '';
  const password = typeof body.password === 'string' ? body.password : '';
  if (!username || !password) return new Response(null, { status: 400 });

  return proxyId(ctx, '/api/v1/me/username', {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
};
