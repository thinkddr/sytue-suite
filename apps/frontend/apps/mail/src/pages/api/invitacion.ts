/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/invitacion  { emailId, partstat }  → { uid, partstat, replySent }
 *
 * CONTESTAR DESDE EL LECTOR LA INVITACIÓN DE UN ORGANIZADOR DE FUERA (D-MAIL-04).
 * El correo trae el REQUEST iMIP; Sytue Calendar lo guarda en el calendario por
 * defecto con la respuesta y manda el REPLY a quien organiza
 * (`POST /api/v1/invitations/import`).
 *
 * EL iCALENDAR SE LEE AQUÍ, DEL MENSAJE, y no se acepta del navegador: así lo que
 * entra en el calendario es lo que de verdad llegó al buzón de quien contesta. Quién
 * contesta lo decide Calendar con el token (sus buzones), no este cuerpo.
 */
import type { APIRoute } from 'astro';
import { CalendarClient, HttpError, bearer } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';
import { esRespuesta, leerInvitacion } from '../../lib/invitacion.js';

export const prerender = false;

const env = serverEnv(import.meta.env);

function calendarApiUrl(): string {
  // El Service del clúster primero: desde un pod, el nombre público no resuelve.
  return (
    env.SYTUE_CALENDAR_INTERNAL_URL ??
    env.SYTUE_CALENDAR_API_URL ??
    env.PUBLIC_SYTUE_CALENDAR_BASE_URL ??
    'https://calendar.sytue.eu'
  );
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { emailId?: unknown; partstat?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const emailId = typeof body.emailId === 'string' ? body.emailId : '';
  if (!emailId || !esRespuesta(body.partstat)) return json({ error: 'invalid_body' }, 400);

  let ics: string | undefined;
  try {
    ics = (await jmapClient(session.accessToken).getMessageBody(emailId)).calendar;
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
  if (!ics || !leerInvitacion(ics)) return json({ error: 'not_an_invitation' }, 404);

  const calendar = new CalendarClient({
    baseUrl: calendarApiUrl(),
    auth: bearer(session.accessToken),
  });
  try {
    const r = await calendar.importInvitation(ics, body.partstat);
    return json({ uid: r.uid, partstat: r.partstat, replySent: r.replySent });
  } catch (err) {
    // 404: quien contesta no figura entre los invitados (un correo reenviado).
    if (err instanceof HttpError && (err.status === 404 || err.status === 400)) {
      return json({ error: err.status === 404 ? 'not_invited' : 'invalid_invitation' }, err.status);
    }
    return json({ error: 'calendar_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
