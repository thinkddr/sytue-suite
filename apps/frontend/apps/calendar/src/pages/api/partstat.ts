/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/partstat — responder a una invitación DESDE EL PROPIO EVENTO.
 *
 * El servicio expone `PUT /api/v1/events/{id}/attendees/partstat` desde hace
 * tiempo y no lo llamaba nadie: solo se podía responder desde la bandeja de
 * invitaciones. Y ahí está el problema, porque quien abre una invitación casi
 * nunca viene de la bandeja: viene de ver el hueco en su semana y pulsarlo. Al
 * llegar al detalle no había nada que responder, así que había que salir, buscar
 * la bandeja y encontrar el mismo evento otra vez.
 *
 * `PARTSTAT` es el nombre del campo en iCalendar (RFC 5545), y se usa aquí y no
 * un «estado» propio: es lo que viaja en el .ics y lo que entiende cualquier otro
 * calendario del mundo.
 *
 * Proxy BFF autorizado por cookie: el access token httpOnly autoriza la API REST
 * del servicio Go y nunca llega al navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { CALENDAR_BASE_URL } from '../../lib/caldav.js';

export const prerender = false;

/** Las tres respuestas del RFC 5545 que una persona puede dar. */
const ESTADOS = new Set(['accepted', 'declined', 'tentative']);

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { id?: unknown; status?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const id = typeof body.id === 'string' ? body.id.trim() : '';
  const status = typeof body.status === 'string' ? body.status.trim().toLowerCase() : '';
  if (!id) return json({ error: 'missing_id' }, 400);
  // Se valida aquí y no solo en el servicio para que un estado inventado no llegue
  // a escribirse en el .ics de nadie: un PARTSTAT desconocido lo ignoran unos
  // clientes y otros lo pintan en crudo.
  if (!ESTADOS.has(status)) return json({ error: 'invalid_status' }, 400);

  try {
    const res = await fetch(
      `${CALENDAR_BASE_URL}/api/v1/events/${encodeURIComponent(id)}/attendees/partstat`,
      {
        method: 'PUT',
        headers: {
          authorization: `Bearer ${session.accessToken}`,
          'content-type': 'application/json',
          accept: 'application/json',
        },
        body: JSON.stringify({ partstat: status.toUpperCase() }),
      },
    );
    if (!res.ok) {
      // 403 y 404 se propagan tal cual: «no estás invitado a ese evento» y «ese
      // evento no existe» son cosas distintas y quien llama debe poder decirlas.
      return json(
        { error: 'partstat_failed' },
        res.status >= 400 && res.status < 500 ? res.status : 502,
      );
    }
    return json({ ok: true, status });
  } catch {
    return json({ error: 'calendar_unreachable' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
