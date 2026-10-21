/** DELETE /api/notes/{id}/purge — borra una nota de la papelera para siempre. */
import type { APIRoute } from 'astro';
import { conCliente, fallo } from '../../../../lib/notes.js';

export const prerender = false;

export const DELETE: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    await c.client.purgeNote(ctx.params.id ?? '');
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
