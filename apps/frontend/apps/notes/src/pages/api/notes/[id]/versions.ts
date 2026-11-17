/** GET /api/notes/{id}/versions — el historial de versiones de una nota. */
import type { APIRoute } from 'astro';
import { conCliente, fallo, json } from '../../../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json({ versions: await c.client.noteVersions(ctx.params.id ?? '') });
  } catch (err) {
    return fallo(err);
  }
};
