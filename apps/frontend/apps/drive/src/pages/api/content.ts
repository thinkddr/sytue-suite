/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — contenido de archivos.
 *
 *   GET /api/content?path=/ruta                       Descarga un archivo (proxy del blob).
 *   GET /api/content?path=/ruta&disposition=inline    Sirve el archivo para vista previa.
 *                                                     Admite Range (vídeo y audio).
 *   PUT /api/content?path=/ruta                       Sube/sobrescribe un archivo (cuerpo = bytes).
 *
 * Proxy sobre el servicio Go propio (drive.sytue.eu) via DriveClient con el access
 * token de la sesion httpOnly. El token NUNCA llega al cliente.
 */
import type { APIRoute } from 'astro';
import { HttpError, upstreamStatus } from '@sytue/api-clients';
import { driveClientFor } from '../../lib/auth';
import { respuestaDeContenido } from '../../lib/respuesta-contenido';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Nombre de fichero seguro para la cabecera Content-Disposition. */
function basename(path: string): string {
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] || 'descarga';
}

export const GET: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const path = url.searchParams.get('path');
  if (!path) return json({ error: 'missing_path' }, 400);

  try {
    // STREAMING: se reenvía el cuerpo del servicio sin leerlo (memoria O(1)). Antes se
    // materializaba el fichero ENTERO en un Blob antes de contestar el primer byte, así
    // que descargar 2 GB reservaba 2 GB en el app-server —compartido por todos los
    // usuarios— y retrasaba el arranque de la descarga hasta tenerlo todo.
    // Range/If-Range se reenvían: <video> y <audio> piden tramos para buscar.
    const res = await client.openDownload(path, {
      range: ctx.request.headers.get('range'),
      ifRange: ctx.request.headers.get('if-range'),
    });
    const name = basename(path).replace(/["\\]/g, '');
    const { status, headers } = respuestaDeContenido(res, {
      name,
      inline: url.searchParams.get('disposition') === 'inline',
    });
    return new Response(res.body, { status, headers });
  } catch (e) {
    return json({ error: 'download_failed' }, upstreamStatus(e));
  }
};

export const PUT: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const path = url.searchParams.get('path');
  if (!path) return json({ error: 'missing_path' }, 400);

  // El tipo real viaja en ?type= (el hilo sube octet-stream para no disparar la
  // protección CSRF de Astro con content-types "de formulario" tras el proxy).
  const contentType =
    url.searchParams.get('type') ||
    ctx.request.headers.get('content-type') ||
    'application/octet-stream';
  try {
    const data = await ctx.request.arrayBuffer();
    const resource = await client.upload(path, data, contentType);
    return json({ resource }, 201);
  } catch (e) {
    // Preserva los 4xx del servicio y, además, el 507 de cuota superada (>= 500 pero
    // es un error del cliente, no del servidor): la UI de subida lo muestra claro.
    const status =
      e instanceof HttpError && ((e.status >= 400 && e.status < 500) || e.status === 507)
        ? e.status
        : 502;
    return json({ error: 'upload_failed' }, status);
  }
};
