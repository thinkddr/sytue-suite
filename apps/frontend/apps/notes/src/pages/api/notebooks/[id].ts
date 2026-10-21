/**
 * GET|PUT|DELETE /api/notebooks/{id} — un cuaderno: leerlo, cambiar sus ajustes
 * (nombre, color, disposición, visibilidad, índice, cifrado, enlace público) o
 * mandarlo a la papelera con su rama.
 */
import type { APIRoute } from 'astro';
import type { NotebookInput } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json(await c.client.getNotebook(ctx.params.id ?? ''));
  } catch (err) {
    return fallo(err);
  }
};

export const PUT: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json(
      await c.client.updateNotebook(ctx.params.id ?? '', await cuerpo<NotebookInput>(ctx)),
    );
  } catch (err) {
    return fallo(err);
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    await c.client.deleteNotebook(ctx.params.id ?? '');
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
