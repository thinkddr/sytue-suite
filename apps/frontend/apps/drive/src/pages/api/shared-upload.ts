/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — subir a una carpeta compartida (solo editor).
 *
 *   PUT /api/shared-upload?id=<raíz>&subpath=<carpeta>&name=<fichero>&type=<mime>
 *       (cuerpo = bytes)   → { resource: DriveResource } | 403 si viewer
 *
 * Proxya la subida gated del servicio Go: la verja de acceso exige permiso de
 * escritura (un viewer recibe 403; sin share vigente, 404). El token NUNCA llega al
 * cliente. La mutación va por POST al servicio con el nombre en ?name= (el hilo sube
 * octet-stream para no disparar la protección CSRF de Astro tras el proxy).
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const PUT: APIRoute = async (ctx) => {
  const url = new URL(ctx.request.url);
  const id = url.searchParams.get('id')?.trim() || '';
  const name = url.searchParams.get('name')?.trim() || '';
  if (!id) return json({ error: 'missing_id' }, 400);
  if (!name) return json({ error: 'missing_name' }, 400);
  const subpath = url.searchParams.get('subpath') ?? '';
  const contentType = url.searchParams.get('type') || 'application/octet-stream';

  const qs = new URLSearchParams({ name });
  if (subpath) qs.set('subpath', subpath);
  const data = await ctx.request.arrayBuffer();
  const res = await driveFetch(ctx, `/api/v1/shared/${encodeURIComponent(id)}/files?${qs}`, {
    method: 'POST',
    headers: { 'content-type': contentType },
    body: data,
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'upload_failed' }, res.status >= 500 ? 502 : res.status);
  const resource = (await res.json()) as unknown;
  return json({ resource }, 201);
};
