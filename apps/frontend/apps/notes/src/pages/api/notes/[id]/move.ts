/**
 * POST /api/notes/{id}/move — reubica una nota (lienzo libre, columna o lista).
 * Endpoint aparte del de edición: arrastrar es la escritura más frecuente del
 * producto y no tiene por qué reenviar el contenido en cada soltar.
 */
import type { APIRoute } from 'astro';
import { conCliente, cuerpo, fallo } from '../../../../lib/notes.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  const pos = await cuerpo<{
    x: number;
    y: number;
    col: number;
    position: number;
    group_id?: string | null;
    /** Con cuaderno de destino, el movimiento es de cuaderno y solo eso. */
    notebook_id?: string;
  }>(ctx);
  try {
    if (pos.notebook_id) await c.client.moveNoteToNotebook(ctx.params.id ?? '', pos.notebook_id);
    else await c.client.moveNote(ctx.params.id ?? '', pos);
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
