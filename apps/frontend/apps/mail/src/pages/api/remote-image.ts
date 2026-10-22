/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/remote-image?url=https://… → descarga una imagen remota de un correo
 * a través del servidor cuando la persona pulsa «Mostrar imágenes» (FE-03).
 *
 * Exige sesión activa, aplica anti-SSRF (solo https en puerto 443 e IP públicas
 * en cada salto), rechaza todo lo que no sea `image/*` rasterizada (sin SVG) y
 * corta la respuesta si supera `TOPE_BYTES_IMAGEN_REMOTA`.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { descargarImagenRemota } from '../../lib/remoteImage.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const rawUrl = ctx.url.searchParams.get('url')?.trim() ?? '';
  if (!rawUrl) return json({ error: 'missing_url' }, 400);

  const res = await descargarImagenRemota(rawUrl, { signal: ctx.request.signal });
  if (!res.ok) {
    return json({ error: res.error }, res.status);
  }

  const buf = new ArrayBuffer(res.bytes.byteLength);
  new Uint8Array(buf).set(res.bytes);
  return new Response(buf, {
    headers: {
      'content-type': res.contentType,
      'content-length': String(res.bytes.byteLength),
      'cache-control': 'private, max-age=300',
      'x-content-type-options': 'nosniff',
      'content-security-policy': "default-src 'none'; sandbox",
    },
  });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
