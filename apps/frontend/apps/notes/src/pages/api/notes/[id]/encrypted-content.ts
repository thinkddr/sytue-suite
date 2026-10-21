/** CAS del contenido cifrado; el BFF nunca recibe el claro. */
import type { APIRoute } from 'astro';
import type { NoteCipherMigration } from '@sytue/api-clients';
import { conCliente, cuerpo, fallo, json } from '../../../../lib/notes.js';

export const prerender = false;
export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  const body = await cuerpo<NoteCipherMigration>(ctx);
  try {
    return json(await c.client.migrateEncryptedContent(ctx.params.id ?? '', body));
  } catch (err) {
    return fallo(err);
  }
};
