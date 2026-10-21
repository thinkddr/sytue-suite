/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/eventos — el flujo SSE de llegadas, con la sesión puesta.
 *
 * POR QUÉ HACE FALTA ESTE PROXY. El servicio Go exige `Authorization: Bearer` y
 * `EventSource` no deja poner cabeceras desde el navegador. La sesión vive aquí,
 * en una cookie httpOnly, así que este endpoint es quien puede convertirla en el
 * token. Es el mismo motivo por el que existen los demás `pages/api/*`.
 *
 * NO SE LEE NI SE TRANSFORMA NADA: se devuelve el cuerpo tal cual llega. Un SSE
 * es un flujo que no termina, y consumirlo aquí para volver a emitirlo sería
 * cortar el tiempo real que se viene a dar.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailOrigin } from '../../lib/mailrest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response('unauthenticated', { status: 401 });

  // `ctx.request.signal`: cuando el navegador cierra la pestaña, se corta también
  // la conexión de arriba. Sin esto quedaría un flujo huérfano por cada pestaña
  // cerrada, y el servicio los iría acumulando sin que nadie los lea.
  const arriba = await fetch(`${mailOrigin()}/jmap/eventos`, {
    headers: { authorization: `Bearer ${session.accessToken}`, accept: 'text/event-stream' },
    signal: ctx.request.signal,
  });
  if (!arriba.ok || !arriba.body) {
    return new Response('mail_stream_error', { status: 502 });
  }

  return new Response(arriba.body, {
    status: 200,
    headers: {
      'content-type': 'text/event-stream; charset=utf-8',
      'cache-control': 'no-cache, no-transform',
      connection: 'keep-alive',
      // Que ningún proxy de en medio lo acumule: sin esto el «tiempo real» llega
      // cuando se llena un búfer, o sea tarde.
      'x-accel-buffering': 'no',
    },
  });
};
