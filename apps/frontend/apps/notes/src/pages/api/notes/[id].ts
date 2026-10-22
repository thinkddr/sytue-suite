/** PUT|DELETE /api/notes/{id} — editar una nota o mandarla a la papelera. */
import type { APIRoute } from 'astro';
import type { NoteInput } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../../lib/notes.js';

export const prerender = false;

async function update(ctx: Parameters<APIRoute>[0], requireEncrypted: boolean): Promise<Response> {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    const body = await cuerpo<NoteInput & { notebook_id: string }>(ctx);
    if (requireEncrypted && (body?.cipher_version !== 2 || !body.expected_crypto))
      return json({ error: 'invalid_encrypted_note' }, 400);
    return json(await c.client.updateNote(ctx.params.id ?? '', body.notebook_id, body));
  } catch (err) {
    return fallo(err);
  }
}

export const PUT: APIRoute = (ctx) => update(ctx, false);
export const encryptedPUT: APIRoute = (ctx) => update(ctx, true);

export const DELETE: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    await c.client.deleteNote(ctx.params.id ?? '');
    return new Response(null, { status: 204 });
  } catch (err) {
    return fallo(err);
  }
};
