/** PUT|DELETE /api/tags/{id} — editar o eliminar una etiqueta con sus estados. */
import type { APIRoute } from 'astro';
import type { TagState } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../../lib/notes.js';

export const prerender = false;

export const PUT: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  const b = await cuerpo<{ name: string; states: TagState[] }>(ctx);
  try {
    return json(await c.client.saveTag(b, ctx.params.id ?? ''));
  } catch (err) {
    return fallo(err);
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    await c.client.deleteTag(ctx.params.id ?? '');
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
