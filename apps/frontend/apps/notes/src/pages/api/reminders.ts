/** GET /api/reminders — mis recordatorios pendientes (el indicador de cada fila). */
import type { APIRoute } from 'astro';
import { conCliente, fallo, json } from '../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json({ reminders: await c.client.listReminders() });
  } catch (err) {
    return fallo(err);
  }
};
