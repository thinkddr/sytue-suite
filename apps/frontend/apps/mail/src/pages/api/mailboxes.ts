/**
 * GET  /api/mailboxes — lista las carpetas/etiquetas JMAP del usuario.
 * POST /api/mailboxes — gestiona una carpeta propia, según el cuerpo:
 *   { name }              → crear
 *   { id, name }          → renombrar
 *   { id, delete: true }  → borrar (solo si está vacía)
 * Proxy BFF: el navegador llama con cookie de sesion; el servidor reenvia al
 * servicio Go Sytue Mail con el access token. Nunca expone el token al cliente.
 *
 * El alta va por REST crudo (mailPost) y no por el SDK: el servicio la acepta
 * desde el primer dia (POST /api/v1/mailboxes) pero @sytue/api-clients no la
 * cubre, y ese paquete es compartido. Renombrar y borrar SÍ van por el SDK
 * (Mailbox/set), porque el servicio solo los ofrece por JMAP.
 *
 * Todo por POST, incluido el borrado: el checkOrigin CSRF de Astro rechaza
 * PUT/DELETE con cuerpo desde el navegador (mismo trato que /api/labels).
 */
import type { APIRoute } from 'astro';
import type { MailboxSetError } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';
import { mailPost } from '../../lib/mailrest.js';

export const prerender = false;

interface Payload {
  /** Presente al renombrar o borrar; ausente al crear. */
  id?: string;
  name?: string;
  delete?: boolean;
}

/**
 * Traduce el motivo del servidor a un código que la pantalla sabe explicar.
 * Un fallo de carpeta casi nunca es una avería —es del sistema, tiene correo
 * dentro o el nombre ya existe— y decir solo «no se pudo» deja a la persona sin
 * saber qué cambiar.
 */
const SET_ERROR_STATUS: Record<string, number> = {
  alreadyExists: 409,
  forbidden: 403,
  mailboxHasEmail: 409,
  invalidProperties: 400,
  notFound: 404,
};

function setErrorResponse(err: MailboxSetError): Response | null {
  if (err === null) return null;
  return json({ error: err }, SET_ERROR_STATUS[err] ?? 502);
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const mailboxes = await jmapClient(session.accessToken).listMailboxes();
    return json({ mailboxes });
  } catch (err) {
    return json({ error: 'jmap_error', message: String(err) }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  const name = (body.name ?? '').trim();

  if (body.id) {
    try {
      const client = jmapClient(session.accessToken);
      const failed = body.delete
        ? await client.deleteMailbox(body.id)
        : name
          ? await client.renameMailbox(body.id, name)
          : 'invalidProperties';
      return setErrorResponse(failed) ?? json({ ok: true });
    } catch (err) {
      return json({ error: 'jmap_error', message: String(err) }, 502);
    }
  }

  if (!name) return json({ error: 'invalid_payload' }, 400);

  try {
    const res = await mailPost(session.accessToken, '/api/v1/mailboxes', { name });
    // El 409 («ya existe») y el 400 («nombre no válido») se propagan tal cual:
    // son respuestas que la pantalla sabe explicar, no fallos del puente.
    if (res.status === 409) return json({ error: 'exists' }, 409);
    if (!res.ok)
      return json({ error: 'mail_error', status: res.status }, res.status === 400 ? 400 : 502);
    return json(await res.json(), 201);
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
