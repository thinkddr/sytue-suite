/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — contenido de archivos.
 *
 *   GET /api/content?path=/ruta                       Descarga un archivo (proxy del blob).
 *   GET /api/content?path=/ruta&disposition=inline    Sirve el archivo para vista previa.
 *   PUT /api/content?path=/ruta                       Sube/sobrescribe un archivo (cuerpo = bytes).
 *
 * Proxy sobre el servicio Go propio (drive.sytue.eu) via DriveClient con el access
 * token de la sesion httpOnly. El token NUNCA llega al cliente.
 */
import type { APIRoute } from 'astro';
import { HttpError, upstreamStatus } from '@sytue/api-clients';
import { driveClientFor } from '../../lib/auth';

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
    const res = await client.openDownload(path);
    const name = basename(path).replace(/["\\]/g, '');
    // Content-type real del backend (el servicio Go guarda el tipo al subir).
    let type = res.headers.get('content-type') || 'application/octet-stream';
    const headers: Record<string, string> = {
      'cache-control': 'no-store',
      'x-content-type-options': 'nosniff',
    };
    if (url.searchParams.get('disposition') === 'inline') {
      // Solo tipos seguros se sirven inline. Todo texto baja como text/plain:
      // servir text/html en el origen de la app con cookies seria XSS almacenado.
      if (type.startsWith('text/') || type === 'application/json')
        type = 'text/plain; charset=utf-8';
      const safe =
        type.startsWith('image/') || type === 'application/pdf' || type.startsWith('text/plain');
      headers['content-disposition'] = safe
        ? `inline; filename="${name}"`
        : `attachment; filename="${name}"`;
      // Cinturon para navegacion directa (p.ej. SVG con <script>): documento
      // sandbox sin scripts ni origen; no afecta a <img>/<pre> y el visor PDF
      // del navegador renderiza igual.
      if (safe) headers['content-security-policy'] = 'sandbox';
    } else {
      headers['content-disposition'] = `attachment; filename="${name}"`;
    }
    headers['content-type'] = type;
    // El service worker nativo exige tamaño explícito para aplicar de verdad el
    // límite offline de 50 MiB. Al transmitir ya no hay Blob del que sacarlo: se
    // propaga el Content-Length que el servicio Go fija desde el metadato (size_bytes).
    const length = res.headers.get('content-length');
    if (length) headers['content-length'] = length;
    return new Response(res.body, { headers });
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
