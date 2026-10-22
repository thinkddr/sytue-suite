/**
 * GET /api/meeting-detail?id=… — quién está dentro y qué se grabó de una reunión.
 *
 * Las dos lecturas van juntas porque el panel de detalle las pinta a la vez: separarlas
 * serían dos viajes para una sola vista. `present=0` incluye a quien ya salió (el
 * historial de la reunión), útil cuando la reunión terminó y solo queda el registro.
 *
 * Proxy BFF puro hacia el servicio Go de Sytue Rooms; el access token no llega al
 * navegador y la RLS del tenant la aplica Go.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { meetClient } from '../../lib/meet.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = ctx.url.searchParams.get('id')?.trim();
  if (!id) return json({ error: 'missing_id' }, 400);
  const present = ctx.url.searchParams.get('present') !== '0';

  const meet = meetClient({ accessToken: session.accessToken });
  try {
    // En paralelo: son independientes y la vista las necesita a la vez.
    const [participants, recordings] = await Promise.all([
      meet.meetingParticipants(id, present),
      meet.meetingRecordings(id),
    ]);
    return json({ participants, recordings });
  } catch (err) {
    return json({ error: 'meet_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
