// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import type { APIRoute } from 'astro';
import { conCliente, cuerpo, fallo, json } from '../../lib/notes.js';

export const prerender = false;

/**
 * GET /api/daily?from=&to=[&notes=1] — los días con notas del tramo (puntos de la
 * semana, vistas Semana y Días). GET /api/daily?this_day=AAAA-MM-DD — «Este día».
 */
export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  const q = new URL(ctx.request.url).searchParams;
  try {
    const esteDia = q.get('this_day');
    const days = esteDia
      ? await c.client.thisDay(esteDia)
      : await c.client.dailyRange(q.get('from') ?? '', q.get('to') ?? '', q.get('notes') === '1');
    return json({ days });
  } catch (err) {
    return fallo(err);
  }
};
export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    const input = await cuerpo<{ date: string; title: string; carry?: boolean }>(ctx);
    return json(await c.client.dailyNotebook(input.date, input.title, input.carry === true));
  } catch (err) {
    return fallo(err);
  }
};
