/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — «Favoritos» (marca personal por-usuario).
 *
 *   GET  /api/favorites                         → { items: DriveResource[] }
 *   POST /api/favorites { id, kind, favorite }  → marca/desmarca (toggle)
 *
 * El servicio Go resuelve la RUTA de cada favorito, así que los items son
 * DriveResource completos. El toggle va por POST (CSRF: mutación con cuerpo JSON,
 * que no dispara el checkOrigin de Astro). El token NUNCA llega al cliente.
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
  const res = await driveFetch(ctx, '/api/v1/favorites');
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'favorites_failed' }, res.status >= 500 ? 502 : res.status);
  const body = (await res.json()) as { items?: RawNode[] };
  const items: DriveResource[] = (body.items ?? []).map(toResource);
  return json({ items });
};

export const POST: APIRoute = async (ctx) => {
  let body: { id?: unknown; kind?: unknown; favorite?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const id = typeof body.id === 'string' ? body.id.trim() : '';
  if (!id) return json({ error: 'missing_id' }, 400);
  const target = body.kind === 'folder' ? { folder_id: id } : { file_id: id };

  const res = await driveFetch(ctx, '/api/v1/favorites', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ ...target, favorite: body.favorite === true }),
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'favorite_failed' }, res.status >= 500 ? 502 : res.status);
  return json({ ok: true });
};
