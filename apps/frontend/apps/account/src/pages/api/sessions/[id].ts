/**
 * Cerrar UNA sesión concreta del propio usuario.
 * Proxy hacia `DELETE /api/v1/me/sessions/{id}` de Sytue ID por el BFF, para
 * que el access token no salga al navegador.
 *
 * Existía el botón y el manejador del IdP (`handleMeSessionRevoke`) pero no la
 * ruta del BFF: el DELETE del navegador caía en /api/sessions (revocar TODAS
 * las demás) con un 404 en la consola — y quien cerraba una sesión estaba
 * cerrando todas las suyas sin saberlo.
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../../lib/bff.js';

export const prerender = false;

export const DELETE: APIRoute = async (ctx) => {
  const { id } = ctx.params;
  if (!id)
    return new Response(JSON.stringify({ error: 'missing id' }), {
      status: 400,
      headers: { 'content-type': 'application/json' },
    });
  return proxyId(ctx, `/api/v1/me/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
};
