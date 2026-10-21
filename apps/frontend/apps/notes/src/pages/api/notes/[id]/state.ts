/** POST /api/notes/{id}/state — marca (o quita) un estado de etiqueta. */
import type { APIRoute } from 'astro';
import { conCliente, cuerpo, fallo } from '../../../../lib/notes.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  const b = await cuerpo<{ tag_id: string; state_id: string | null }>(ctx);
  try {
    await c.client.setNoteState(ctx.params.id ?? '', b.tag_id, b.state_id ?? null);
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
