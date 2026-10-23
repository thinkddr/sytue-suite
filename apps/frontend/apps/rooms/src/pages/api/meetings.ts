/**
 * GET /api/meetings — reuniones del tenant (activas por defecto).
 * POST /api/meetings — convoca una reunión SUELTA (sin canal detrás) y entra.
 *
 * Proxy BFF puro (autorizado por cookie) hacia el servicio Go de Sytue Rooms. El
 * access token de Sytue ID nunca llega al navegador. CERO lógica de negocio: crear,
 * listar y unirse los decide Go. docs/CONVENTIONS.md §8.
 *
 * La diferencia con `/api/call` es de dónde nace la sala: allí de un canal (y se
 * reutiliza la suya), aquí de nadie — es el equivalente a «Nueva reunión» de Meet o
 * Zoom, para convocar por enlace a quien no está en un canal.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { meetClient } from '../../lib/meet.js';

import { isMatrixDeviceId } from '../../client/matrix/types.js';
export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  // `status` acota el ciclo de vida. Por defecto `active`: es lo único a lo que uno
  // puede unirse, y es lo que la lista ofrece con un botón «Entrar».
  const raw = ctx.url.searchParams.get('status')?.trim() || 'active';
  const status = raw === 'scheduled' || raw === 'ended' || raw === 'active' ? raw : 'active';

  try {
    const meetings = await meetClient({ accessToken: session.accessToken }).listMeetings(status);
    return json({ meetings });
  } catch {
    return json({ error: 'meet_error' }, 502);
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { title?: string; matrixDeviceId?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  // El servicio Go exige título no vacío. Si el usuario no puso ninguno, se compone
  // uno reconocible en la lista («Reunión de Ada»); saneo del input, no negocio.
  const title = payload.title?.trim() || `Reunión de ${session.user.name ?? 'Sytue'}`;

  const device = payload.matrixDeviceId;
  if (!isMatrixDeviceId(device)) return json({ error: 'missing_matrix_device_id' }, 400);
  try {
    const call = await meetClient({ accessToken: session.accessToken }).createMeeting(
      title,
      session.user.name,
      device,
    );
    return json(call, 201);
  } catch {
    return json({ error: 'meet_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
