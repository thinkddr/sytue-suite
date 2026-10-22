// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/** POST /api/notes/{id}/postpone {date,title} — «Posponer»: al diario de ese día. */
import type { APIRoute } from 'astro';
import { conCliente, cuerpo, fallo, json } from '../../../../lib/notes.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  try {
    const input = await cuerpo<{ date: string; title: string }>(ctx);
    return json(await c.client.postponeTask(ctx.params.id ?? '', input.date, input.title));
  } catch (err) {
    return fallo(err);
  }
};
