/**
 * GET /api/body?emailId= → cuerpo completo de un correo leído (JSON {text, html}).
 *
 * El sobre indexado (Email/get) solo trae `preview` (extracto truncado por el
 * servicio Go): el SDK descarga el .eml crudo y extrae las partes text/plain y
 * text/html. Proxy BFF autorizado por cookie; el access token nunca llega al
 * cliente. El HTML se devuelve crudo: el lector lo aísla en un iframe sandbox.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const emailId = ctx.url.searchParams.get('emailId');
  if (!emailId) return json({ error: 'missing_emailId' }, 400);

  try {
    const body = await jmapClient(session.accessToken).getMessageBody(emailId);
    return json(body);
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
