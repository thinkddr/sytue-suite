/**
 * Contraseñas de aplicación: listar y crear.
 *
 * La respuesta del alta trae la contraseña EN CLARO y es la única vez que
 * existe: solo se guarda su hash. Por eso no se cachea nada aquí y la pantalla la
 * enseña hasta que quien la ha creado confirma que la ha guardado.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => proxyId(ctx, '/api/v1/me/app-passwords');

export const POST: APIRoute = async (ctx) => {
  let label = '';
  try {
    const body = (await ctx.request.json()) as { label?: unknown };
    label = typeof body.label === 'string' ? body.label.trim() : '';
  } catch {
    return new Response(null, { status: 400 });
  }
  return proxyId(ctx, '/api/v1/me/app-passwords', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ label }),
  });
};
