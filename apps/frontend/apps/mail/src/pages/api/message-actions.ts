/**
 * POST /api/message-actions — triaje de un correo: estrella ($flagged) o mover
 * de carpeta (archivar/papelera/«Mover a…»). Proxy puro al servicio Go de correo
 * (POST /api/v1/messages/{id}/flags | /move). Siempre POST: el checkOrigin CSRF
 * de Astro rechaza DELETE/PATCH con cuerpo desde el navegador.
 * Cuerpo JSON: { emailId, flagged: boolean } XOR { emailId, moveTo: mailboxId }.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';

export const prerender = false;

interface Payload {
  emailId?: string;
  flagged?: boolean;
  moveTo?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  const isFlag = typeof body.flagged === 'boolean';
  const isMove = typeof body.moveTo === 'string' && body.moveTo.length > 0;
  // Exactamente una acción por petición (ni ninguna ni ambas).
  if (!body.emailId || isFlag === isMove) {
    return json({ error: 'invalid_payload' }, 400);
  }
  try {
    const client = jmapClient(session.accessToken);
    if (isFlag) await client.setFlagged(body.emailId, body.flagged as boolean);
    else await client.moveMessage(body.emailId, body.moveTo as string);
    return json({ ok: true });
  } catch (err) {
    return json({ error: 'jmap_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
