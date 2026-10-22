/** POST|DELETE /api/notebooks/{id}/archive — archivar la rama, o devolverla al árbol. */
import type { APIRoute } from 'astro';
import { conCliente, fallo } from '../../../../lib/notes.js';

export const prerender = false;

const archivar =
  (si: boolean): APIRoute =>
  async (ctx) => {
    const c = await conCliente(ctx);
    if (c instanceof Response) return c;
    try {
      await c.client.archiveNotebook(ctx.params.id ?? '', si);
      return new Response(null, { status: 204 });
    } catch (err) {
      return fallo(err);
    }
  };

export const POST: APIRoute = archivar(true);
export const DELETE: APIRoute = archivar(false);
