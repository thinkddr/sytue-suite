/** GET /api/notes/{id}/versions/{version} — una versión entera de la nota. */
import type { APIRoute } from 'astro';
import { conCliente, fallo, json } from '../../../../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json(await c.client.noteVersion(ctx.params.id ?? '', ctx.params.version ?? ''));
  } catch (err) {
    return fallo(err);
  }
};
