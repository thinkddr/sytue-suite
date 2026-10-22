/**
 * POST /api/batch — acciones en LOTE sobre una multiselección (o un hilo entero):
 * mover N correos (archivar/papelera/spam/«Mover a…») o marcar N leídos/no leídos,
 * cada una en UNA operación set-based del servicio Go (id = ANY), no N round-trips.
 * Proxy BFF autorizado por cookie; la autorización por dueño la impone el servicio
 * (owner por account_id + RLS por tenant). Siempre POST (checkOrigin CSRF de Astro).
 * Cuerpo JSON (exactamente una acción):
 *   { ids:[…], moveTo: mailboxId }      → mover en lote
 *   { ids:[…], flag: '$seen', add }     → marcar leído/no leído (o $flagged) en lote
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailPost } from '../../lib/mailrest.js';

export const prerender = false;

interface Payload {
  ids?: string[];
  moveTo?: string;
  flag?: string;
  add?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (!Array.isArray(body.ids) || body.ids.length === 0) {
    return json({ error: 'invalid_payload' }, 400);
  }
  const isMove = typeof body.moveTo === 'string' && body.moveTo.length > 0;
  const isFlag = typeof body.flag === 'string' && body.flag.length > 0;
  // Exactamente una acción por petición (ni ninguna ni ambas).
  if (isMove === isFlag) return json({ error: 'invalid_payload' }, 400);

  try {
    const res = isMove
      ? await mailPost(session.accessToken, '/api/v1/messages/batch/move', {
          ids: body.ids,
          mailbox_id: body.moveTo,
        })
      : await mailPost(session.accessToken, '/api/v1/messages/batch/flags', {
          ids: body.ids,
          flag: body.flag,
          add: body.add === true,
        });
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    return json(await res.json());
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
