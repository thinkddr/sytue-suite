/**
 * GET /api/call-active?channelId= — ¿hay una llamada EN CURSO en el canal? Proxy
 * BFF PURO (autorizado por cookie) hacia el servicio Go Sytue Meet: consulta la sala
 * del canal por su origen SIN crearla y cuenta los participantes presentes. Devuelve
 * { participants }. 0 = no hay llamada (o Meet no disponible: degradación limpia).
 * Sondeo ligero (throttled por el cliente); cero escrituras. docs/CONVENTIONS.md §8.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { meetClient } from '../../lib/meet.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const channelId = ctx.url.searchParams.get('channelId')?.trim();
  if (!channelId) return json({ error: 'missing_channelId' }, 400);

  try {
    const active = await meetClient({ accessToken: session.accessToken }).activeCall(channelId);
    return json(active);
  } catch {
    // Degradación limpia: si Meet no responde, el canal simplemente no muestra banner.
    return json({ participants: 0 });
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
