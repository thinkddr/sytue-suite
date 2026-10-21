/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — «Recientes».
 *
 *   GET /api/recent[?cursor=<opaco>&limit=N]   → { items: DriveResource[], nextCursor? }
 *
 * Lista los ficheros del tenant por reciente-primero (updated_at DESC, keyset).
 * El servicio Go resuelve la RUTA de cada fichero (los recientes cruzan todo el
 * árbol), así que los items son DriveResource completos: se abren/descargan/
 * comparten con la misma maquinaria por-ruta del explorador. El token NUNCA llega
 * al cliente.
 */
import type { APIRoute } from 'astro';
import type { DriveResource } from '@sytue/api-clients';
import { driveFetch } from '../../lib/auth';
import { toResource, type RawNode } from '../../lib/nodes';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const params = new URL(ctx.request.url).searchParams;
  const cursor = params.get('cursor')?.trim() || '';
  const limit = params.get('limit')?.trim() || '';
  const qs = new URLSearchParams();
  if (cursor) qs.set('cursor', cursor);
  if (limit) qs.set('limit', limit);
  const suffix = qs.toString() ? `?${qs}` : '';

  const res = await driveFetch(ctx, `/api/v1/recent${suffix}`);
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'recent_failed' }, res.status >= 500 ? 502 : res.status);
  const body = (await res.json()) as { items?: RawNode[]; next_cursor?: string };
  const items: DriveResource[] = (body.items ?? []).map(toResource);
  return json({ items, ...(body.next_cursor ? { nextCursor: body.next_cursor } : {}) });
};
