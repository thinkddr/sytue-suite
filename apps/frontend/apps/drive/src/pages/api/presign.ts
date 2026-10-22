/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — subida directa a S3 (presigned), paso 1: PRESIGN.
 *
 *   POST /api/presign  { path, size, contentType }  → { url, handle, expires_at }
 *
 * Proxya a POST /api/v1/files/presign del servicio Go, que comprueba permiso
 * (acceso a la carpeta destino) y cuota ANTES de firmar y devuelve una URL PUT
 * prefirmada acotada (clave+método+caducidad) + un handle firmado. El cliente sube
 * el fichero DIRECTO a esa URL (a S3, no al app-server) y luego confirma con POST
 * /api/finalize. Es una vía ADICIONAL para ficheros grandes: PUT /api/content sigue
 * de fallback. POST con cuerpo JSON (no dispara el checkOrigin CSRF de Astro tras el
 * proxy). El access token de la sesión httpOnly va server-side (driveFetch); NUNCA
 * llega al cliente.
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

export const POST: APIRoute = async (ctx) => {
  let body: { path?: unknown; size?: unknown; contentType?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const path = typeof body.path === 'string' ? body.path.trim() : '';
  const size =
    typeof body.size === 'number' && Number.isFinite(body.size) ? Math.floor(body.size) : 0;
  const contentType = typeof body.contentType === 'string' ? body.contentType : '';
  if (!path || size <= 0) return json({ error: 'invalid_request' }, 400);

  const res = await driveFetch(ctx, '/api/v1/files/presign', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ path, size, content_type: contentType }),
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) {
    // Preserva los 4xx del servicio y el 507 (cuota): el hilo distingue la cuota
    // para NO caer al fallback (fallaría igual). El resto de 5xx → 502.
    const status = res.status < 500 || res.status === 507 ? res.status : 502;
    return json({ error: 'presign_failed' }, status);
  }
  const text = await res.text();
  return new Response(text || '{}', {
    status: 200,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};
