import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth';

export const prerender = false;

/**
 * La rúbrica que la persona pidió recordar al firmar («Recordar mi firma»).
 *
 *   GET    /api/pdf-rubrica  -> GET    /api/v1/me/rubrica  {trazos} | 404
 *   PUT    /api/pdf-rubrica  -> PUT    /api/v1/me/rubrica  {trazos}
 *   DELETE /api/pdf-rubrica  -> DELETE /api/v1/me/rubrica
 *
 * Vive en la CUENTA (Sytue ID, cifrada) y no en el navegador porque se borra desde
 * Mi cuenta, que es otro origen, y porque firmar en otro equipo no debería obligar
 * a dibujarla otra vez.
 */
const RUTA = '/api/v1/me/rubrica';

async function reenviar(res: Response): Promise<Response> {
  return new Response(res.status === 204 ? null : await res.text(), {
    status: res.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => reenviar(await bff.idApi(toBffContext(ctx), RUTA));

export const PUT: APIRoute = async (ctx) =>
  reenviar(
    await bff.idApi(toBffContext(ctx), RUTA, {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: await ctx.request.text(),
    }),
  );

export const DELETE: APIRoute = async (ctx) =>
  reenviar(await bff.idApi(toBffContext(ctx), RUTA, { method: 'DELETE' }));
