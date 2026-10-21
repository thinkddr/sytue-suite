/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — miniatura server-side de un fichero de imagen.
 *
 *   GET /api/thumbnail?id=<fileId>   Miniatura JPEG/PNG pequeña (proxy del servicio Go).
 *
 * Proxy en STREAMING sobre el servicio Go propio (drive.sytue.eu) con el access
 * token de la sesión httpOnly (driveFetch); el token NUNCA llega al cliente. El
 * servicio Go genera la miniatura perezosamente y cachea la derivada (keyed por
 * checksum), la gatea por permiso (RLS: un fichero ajeno → 404) y valida el tipo
 * (solo imágenes ráster → 415). La disposición es SEGURA para pintarla en un <img>:
 * inline + nosniff + CSP sandbox (una miniatura nunca ejecuta script). Cacheable con
 * ETag: se reenvía If-None-Match y se propaga el 304 (revalidación barata).
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

const CACHE = 'private, max-age=300';

export const GET: APIRoute = async (ctx) => {
  const id = new URL(ctx.request.url).searchParams.get('id')?.trim();
  if (!id) return json({ error: 'missing_id' }, 400);

  const inm = ctx.request.headers.get('if-none-match');
  const res = await driveFetch(
    ctx,
    `/api/v1/files/${encodeURIComponent(id)}/thumbnail`,
    inm ? { headers: { 'if-none-match': inm } } : undefined,
  );
  if (!res) return json({ error: 'unauthenticated' }, 401);

  // Revalidación: el contenido no cambió (mismo ETag) → 304 sin cuerpo.
  if (res.status === 304) {
    const h = new Headers({ 'cache-control': CACHE });
    const etag = res.headers.get('etag');
    if (etag) h.set('etag', etag);
    return new Response(null, { status: 304, headers: h });
  }
  if (!res.ok || !res.body)
    return json({ error: 'thumbnail_failed' }, res.status >= 500 ? 502 : res.status);

  // Solo imágenes inline (defensa en profundidad; el servicio ya valida el tipo).
  const type = res.headers.get('content-type') ?? 'application/octet-stream';
  const headers = new Headers({
    'content-type': type.startsWith('image/') ? type : 'application/octet-stream',
    'cache-control': CACHE,
    'x-content-type-options': 'nosniff',
    'content-disposition': 'inline',
    'content-security-policy': 'sandbox',
  });
  const etag = res.headers.get('etag');
  if (etag) headers.set('etag', etag);
  return new Response(res.body, { status: 200, headers });
};
