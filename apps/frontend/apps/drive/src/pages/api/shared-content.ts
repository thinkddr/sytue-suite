/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — contenido de un fichero DENTRO de un recurso compartido.
 *
 *   GET /api/shared-content?id=<raíz>&subpath=<fichero>                 Descarga.
 *   GET /api/shared-content?id=<raíz>&subpath=<fichero>&disposition=inline  Vista previa.
 *
 * Proxya el contenido gated del servicio Go (verja de acceso por share). Misma
 * política de disposition que /api/content: inline SOLO para tipos seguros
 * (imagen/PDF/texto plano), con CSP sandbox; el resto baja como adjunto. El token
 * NUNCA llega al cliente.
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

/** Nombre de fichero seguro para Content-Disposition (último segmento del subpath). */
function basename(subpath: string): string {
  const parts = subpath.split('/').filter(Boolean);
  return (parts[parts.length - 1] || 'descarga').replace(/["\\]/g, '');
}

export const GET: APIRoute = async (ctx) => {
  const url = new URL(ctx.request.url);
  const id = url.searchParams.get('id')?.trim() || '';
  if (!id) return json({ error: 'missing_id' }, 400);
  const subpath = url.searchParams.get('subpath') ?? '';

  const qs = subpath ? `?subpath=${encodeURIComponent(subpath)}` : '';
  const res = await driveFetch(ctx, `/api/v1/shared/${encodeURIComponent(id)}/content${qs}`);
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'download_failed' }, res.status >= 500 ? 502 : res.status);

  // STREAMING: se reenvía el cuerpo sin leerlo (memoria O(1)). Antes se materializaba el
  // fichero ENTERO en un Blob antes de contestar, así que un share de 2 GB reservaba 2 GB
  // en el app-server. Aquí muerde más que en /api/content: un enlace público lo abre
  // cualquiera, sin sesión, así que el pico de memoria no está acotado por el nº de
  // usuarios del tenant.
  const name = basename(subpath);
  let type = res.headers.get('content-type') || 'application/octet-stream';
  const headers: Record<string, string> = {
    'cache-control': 'no-store',
    'x-content-type-options': 'nosniff',
  };
  if (url.searchParams.get('disposition') === 'inline') {
    // Todo texto baja como text/plain: servir text/html en el origen de la app con
    // cookies sería XSS almacenado.
    if (type.startsWith('text/') || type === 'application/json') type = 'text/plain; charset=utf-8';
    const safe =
      type.startsWith('image/') || type === 'application/pdf' || type.startsWith('text/plain');
    headers['content-disposition'] = safe
      ? `inline; filename="${name}"`
      : `attachment; filename="${name}"`;
    if (safe) headers['content-security-policy'] = 'sandbox';
  } else {
    headers['content-disposition'] = `attachment; filename="${name}"`;
  }
  headers['content-type'] = type;
  // Necesario para que el cliente nativo pueda verificar el límite offline de
  // 50 MiB antes de persistir el fichero: se propaga el del servicio Go.
  const length = res.headers.get('content-length');
  if (length) headers['content-length'] = length;
  return new Response(res.body, { headers });
};
