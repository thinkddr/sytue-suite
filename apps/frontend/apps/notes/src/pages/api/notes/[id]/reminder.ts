/** PUT / DELETE /api/notes/{id}/reminder — pone o quita mi recordatorio en una nota. */
import type { APIRoute } from 'astro';
import type { ReminderInput } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../../../lib/notes.js';

export const prerender = false;

export const PUT: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json(await c.client.setReminder(ctx.params.id ?? '', await cuerpo<ReminderInput>(ctx)));
  } catch (err) {
    return fallo(err);
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    await c.client.deleteReminder(ctx.params.id ?? '');
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
