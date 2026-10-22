// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/** GET /api/tasks?state=open|done|cancelled — la vista «Tareas»: las de todos los cuadernos. */
import type { APIRoute } from 'astro';
import type { TaskState } from '@sytue/api-clients';
import { conCliente, fallo, json } from '../../lib/notes.js';

export const prerender = false;

const ESTADOS: readonly TaskState[] = ['open', 'done', 'cancelled'];

export const GET: APIRoute = async (ctx) => {
  const c = await conCliente(ctx);
  if (c instanceof Response) return c;
  const pedido = ctx.url.searchParams.get('state') ?? 'open';
  const state = ESTADOS.find((e) => e === pedido);
  if (!state) return json({ error: 'invalid_input' }, 400);
  try {
    return json({ tasks: await c.client.listTasks(state) });
  } catch (err) {
    return fallo(err);
  }
};
