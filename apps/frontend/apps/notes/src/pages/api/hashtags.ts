/** GET /api/hashtags — los `#hashtags` de las notas visibles, con cuántas notas lleva cada uno. */
import type { APIRoute } from 'astro';
import { conCliente, fallo, json } from '../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json({ hashtags: await c.client.hashtags() });
  } catch (err) {
    return fallo(err);
  }
};
