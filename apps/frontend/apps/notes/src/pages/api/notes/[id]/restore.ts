/** POST /api/notes/{id}/restore — saca una nota de la papelera. */
import type { APIRoute } from 'astro';
import { conCliente, fallo } from '../../../../lib/notes.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    await c.client.restoreNote(ctx.params.id ?? '');
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
