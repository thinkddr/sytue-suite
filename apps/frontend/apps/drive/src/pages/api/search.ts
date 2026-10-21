/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — BÚSQUEDA por nombre.
 *
 *   GET /api/search?q=<texto>[&limit=N]   → { items: DriveResource[] }
 *
 * El servicio ya la sabía hacer (GET /api/v1/search, y el Hub la federa en la búsqueda
 * global de la topbar), pero el explorador NO tenía caja de búsqueda: buscar un fichero
 * dentro de Drive obligaba a navegar carpeta por carpeta.
 *
 * Cada resultado viene con su RUTA resuelta por el servicio —una búsqueda cruza todo el
 * árbol—, así que se abre, descarga y comparte con la misma maquinaria por-ruta del
 * explorador. El token NUNCA llega al cliente.
 */
import type { APIRoute } from 'astro';
import { consulta } from '@sytue/auth/server';
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

const buscar: APIRoute = async (ctx) => {
  const params = await consulta(ctx.request, ctx.url);
  const q = params.get('q')?.trim() ?? '';
  // Sin término no se llama al servicio: devolvería 400 y el explorador solo necesita
  // saber que no hay nada que enseñar.
  if (!q) return json({ items: [] });

  const qs = new URLSearchParams({ q });
  const limit = params.get('limit')?.trim();
  if (limit) qs.set('limit', limit);

  const res = await driveFetch(ctx, `/api/v1/search?${qs}`);
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'search_failed' }, res.status >= 500 ? 502 : res.status);
  // La envoltura es "files" (la comparte con el conector del Hub), pero los elementos son
  // la MISMA vista plana con ruta que «Recientes»/«Favoritos».
  const body = (await res.json()) as { files?: RawNode[] };
  const items: DriveResource[] = (body.files ?? []).map(toResource);
  return json({ items });
};

// GET y QUERY, el mismo handler. QUERY es el preferente del cliente y GET el
// respaldo para quien no lo hable; ver `consulta` en @sytue/auth/server.
export const GET: APIRoute = buscar;
export const QUERY: APIRoute = buscar;
