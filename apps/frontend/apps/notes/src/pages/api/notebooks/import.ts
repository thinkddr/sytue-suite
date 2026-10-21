/**
 * POST /api/notebooks/import  — crea un cuaderno desde un `.onex`.
 * POST /api/notebooks/import?preview=1 — lo lee y lo devuelve SIN crear nada.
 *
 * La vista previa comparte ruta con la importación a propósito: si aceptara
 * ficheros que la otra rechaza, sería una vista previa de algo que luego no se
 * puede importar.
 */
import type { APIRoute } from 'astro';
import { conCliente, fallo, json } from '../../../lib/notes.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  // El cuerpo se pasa como texto y no como stream: el servicio lo lee entero para
  // interpretarlo como JSON y ya lo corta en 32 MiB, así que no hay nada que
  // ganar reenviándolo a trozos y sí un `duplex` que no todos los runtimes tienen.
  const volcado = await ctx.request.text();
  try {
    if (ctx.url.searchParams.get('preview') === '1') {
      return json(await c.client.previewNotebook(volcado));
    }
    return json(await c.client.importNotebook(volcado), 201);
  } catch (err) {
    return fallo(err);
  }
};
