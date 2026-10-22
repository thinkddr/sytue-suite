/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/bases/<id>/events — el flujo SSE de cambios y presencia de una base, con la
 * sesión puesta.
 *
 * POR QUÉ NO VALE EL PROXY COMÚN (`[...ruta].ts`). Un flujo no termina: hay que cortar
 * la conexión de arriba cuando el navegador cierra la pestaña (`signal`) y pasar el
 * `Last-Event-ID` con el que `EventSource` reanuda donde se quedó. El servicio Go no
 * tiene Ingress —solo se llega a él por aquí—, así que este endpoint es el único camino
 * del tiempo real: convierte la cookie en el token y deja pasar el cuerpo tal cual.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../../../lib/bff.js';
import { DESTINO } from '../../../../lib/servicio.js';

export const prerender = false;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const GET: APIRoute = async (ctx) => {
  const id = ctx.params.id ?? '';
  if (!UUID.test(id)) return new Response(null, { status: 404 });
  const session = await getApiSession(ctx);
  if (!session) return new Response(null, { status: 401 });

  // Solo lo que el contrato admite: la versión y la pestaña. Nada más del cliente sube.
  const q = new URL(ctx.request.url).searchParams;
  const arriba = new URL(`${DESTINO.replace(/\/$/, '')}/api/v1/bases/${id}/events`);
  for (const k of ['v', 'conn']) {
    const v = q.get(k);
    if (v) arriba.searchParams.set(k, v);
  }
  const cabeceras: Record<string, string> = {
    authorization: `Bearer ${session.accessToken}`,
    accept: 'text/event-stream',
  };
  const ultimo = ctx.request.headers.get('last-event-id');
  if (ultimo) cabeceras['last-event-id'] = ultimo;

  const flujo = {
    'content-type': 'text/event-stream; charset=utf-8',
    'cache-control': 'no-cache, no-transform',
    connection: 'keep-alive',
    'x-accel-buffering': 'no',
    'x-content-type-options': 'nosniff',
  };
  // Un fallo se contesta TAMBIÉN como flujo, con su despedida: `EventSource` da por
  // perdida para siempre una respuesta que no sea 200, y así se le puede decir por qué.
  // `access` = no reintentes (sin acceso no hay nada que seguir); `error` = vuelve luego.
  const despedida = (motivo: 'access' | 'error'): Response =>
    new Response(`retry: 10000\nevent: bye\ndata: {"reason":"${motivo}"}\n\n`, {
      status: 200,
      headers: flujo,
    });

  let res: Response;
  try {
    res = await fetch(arriba, { headers: cabeceras, signal: ctx.request.signal });
  } catch {
    return despedida('error');
  }
  if (!res.ok || !res.body) {
    return despedida(res.status === 404 || res.status === 403 ? 'access' : 'error');
  }
  return new Response(res.body, { status: 200, headers: flujo });
};
