/**
 * GET  /api/tags — etiquetas con sus estados.
 * POST /api/tags — crea una etiqueta con TODOS sus estados de una vez (el editor
 *                  es una sola pantalla; guardarla en varias peticiones dejaría
 *                  estados huérfanos si una fallara a mitad).
 */
import type { APIRoute } from 'astro';
import type { TagState } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json({ tags: await c.client.listTags() });
  } catch (err) {
    return fallo(err);
  }
};

export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json(await c.client.saveTag(await cuerpo<{ name: string; states: TagState[] }>(ctx)));
  } catch (err) {
    return fallo(err);
  }
};
