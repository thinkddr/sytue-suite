/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — descarga de una VERSIÓN concreta de un fichero.
 *
 *   GET /api/version-content?id=<fileId>&version=<n>   Descarga el blob de esa versión.
 *
 * Proxy en STREAMING sobre el servicio Go propio (drive.sytue.eu) con el access
 * token de la sesión httpOnly (driveFetch); el token NUNCA llega al cliente. La
 * disposición es SIEMPRE segura (attachment + nosniff): una versión histórica no
 * se sirve inline (evita XSS almacenado por tipo peligroso en el origen de la app).
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

export const GET: APIRoute = async (ctx) => {
  const params = new URL(ctx.request.url).searchParams;
  const id = params.get('id')?.trim();
  const version = params.get('version')?.trim();
  if (!id) return json({ error: 'missing_id' }, 400);
  if (!version || !/^\d+$/.test(version)) return json({ error: 'invalid_version' }, 400);

  const res = await driveFetch(
    ctx,
    `/api/v1/files/${encodeURIComponent(id)}/versions/${version}/content`,
  );
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok || !res.body)
    return json({ error: 'download_failed' }, res.status >= 500 ? 502 : res.status);

  // Reenvía el cuerpo en streaming (sin bufferizar el blob en memoria) con
  // disposición forzada a attachment y nosniff, reutilizando el filename* que ya
  // compone el servicio Go.
  const headers = new Headers({
    'content-type': res.headers.get('content-type') ?? 'application/octet-stream',
    'content-disposition':
      res.headers.get('content-disposition') ?? `attachment; filename="version-${version}"`,
    'x-content-type-options': 'nosniff',
    'cache-control': 'no-store',
  });
  const len = res.headers.get('content-length');
  if (len) headers.set('content-length', len);
  return new Response(res.body, { status: 200, headers });
};
