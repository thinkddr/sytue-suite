/**
 * GET  /api/notebooks/{id}/notes — notas del cuaderno, con estados y enlaces.
 * POST /api/notebooks/{id}/notes — crea una nota dentro.
 */
import type { APIRoute } from 'astro';
import type { NoteInput } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../../../lib/notes.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    return json({ notes: await c.client.listNotes(ctx.params.id ?? '') });
  } catch (err) {
    return fallo(err);
  }
};

async function create(ctx: Parameters<APIRoute>[0], requireEncrypted: boolean): Promise<Response> {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    const input = await cuerpo<NoteInput>(ctx);
    if (requireEncrypted && (input?.cipher_version !== 2 || !input.expected_crypto))
      return json({ error: 'invalid_encrypted_note' }, 400);
    return json(await c.client.createNote(ctx.params.id ?? '', input), 201);
  } catch (err) {
    return fallo(err);
  }
}

export const POST: APIRoute = (ctx) => create(ctx, false);
export const encryptedPOST: APIRoute = (ctx) => create(ctx, true);
