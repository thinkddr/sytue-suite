/**
 * GET  /api/notebooks — árbol de cuadernos visibles.
 * POST /api/notebooks — crea uno.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly (ver src/lib/notes.ts).
 */
import type { APIRoute } from 'astro';
import type { NotebookInput } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json({ notebooks: await c.client.listNotebooks() });
  } catch (err) {
    return fallo(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json(await c.client.createNotebook(await cuerpo<NotebookInput>(ctx)), 201);
  } catch (err) {
    return fallo(err);
  }
};
