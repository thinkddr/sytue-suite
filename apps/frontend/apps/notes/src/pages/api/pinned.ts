/** GET /api/pinned — «Fijadas»: las notas fijadas de todos los cuadernos visibles. */
import type { APIRoute } from 'astro';
import { conCliente, fallo, json } from '../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json({ notes: await c.client.pinnedNotes() });
  } catch (err) {
    return fallo(err);
  }
};
