/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — subida directa a S3 (presigned), paso 2: FINALIZE.
 *
 *   POST /api/finalize  { handle }  → { ...fichero } (201)
 *
 * Proxya a POST /api/v1/files/finalize del servicio Go, que verifica el handle
 * firmado, comprueba en S3 que el objeto llegó y su tamaño (HEAD, sin fiarse del
 * cliente), registra el metadato reservando cuota y ata el blob. Idempotente. POST
 * con cuerpo JSON (no dispara el checkOrigin CSRF de Astro tras el proxy). El access
 * token va server-side (driveFetch); NUNCA llega al cliente.
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
  let body: { handle?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const handle = typeof body.handle === 'string' ? body.handle : '';
  if (!handle) return json({ error: 'missing_handle' }, 400);

  const res = await driveFetch(ctx, '/api/v1/files/finalize', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ handle }),
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) {
    // Preserva 4xx del servicio y el 507 (cuota); otros 5xx → 502.
    const status = res.status < 500 || res.status === 507 ? res.status : 502;
    return json({ error: 'finalize_failed' }, status);
  }
  // Reenvía el metadato del fichero (el hilo no lo usa: recarga la carpeta) con 201.
  const text = await res.text();
  return new Response(text || '{}', {
    status: 201,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};
