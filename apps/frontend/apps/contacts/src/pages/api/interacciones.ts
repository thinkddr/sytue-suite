/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/interacciones?email=a@x.eu&email=b@x.eu — los últimos correos y eventos
 * del usuario con esa persona, por la búsqueda federada del Hub (ver
 * lib/interacciones.ts). El token va al Hub desde aquí y nunca al navegador.
 *
 * Si el Hub no contesta para UNA dirección se sigue con las demás; solo si fallan
 * todas se devuelve 502, para que la ficha diga «no disponible» y no «nada».
 */
import type { APIRoute } from 'astro';
import { HubClient, bearer, type SearchResult } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import {
  MAX_INTERACCIONES,
  ORIGENES_INTERACCIONES,
  correosABuscar,
  respuestaValida,
  unirInteracciones,
} from '../../lib/interacciones.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const HUB = env.SYTUE_HUB_INTERNAL_URL ?? env.SYTUE_HUB_BASE_URL ?? 'https://hub.sytue.eu';

function json(body: unknown, status = 200): Response {
  return Response.json(body, { status, headers: { 'cache-control': 'no-store' } });
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const correos = correosABuscar(ctx.url.searchParams.getAll('email'));
  if (!correos.length) return json({ items: [] });
  const hub = new HubClient({ baseUrl: HUB, auth: bearer(session.accessToken) });
  const respuestas = await Promise.allSettled(
    correos.map((c) =>
      hub.search(c, ORIGENES_INTERACCIONES, { limit: MAX_INTERACCIONES, offset: 0 }),
    ),
  );
  const listas: SearchResult[][] = [];
  for (const r of respuestas)
    if (r.status === 'fulfilled' && respuestaValida(r.value)) listas.push(r.value.results);
  if (!listas.length) return json({ error: 'hub_unavailable' }, 502);
  return json({ items: unirInteracciones(listas) });
};
