/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — almacenamiento de TERCEROS (bucket S3 propio, ADR 0038).
 *
 *   GET    /api/storage-external          → estado (nunca la clave secreta)
 *   PUT    /api/storage-external          → probar, sellar y guardar (el fallo vuelve inline)
 *   DELETE /api/storage-external          → desconectar (409 si quedan ficheros allí)
 *
 * Proxy fino: el servicio Go decide permisos (owner/admin) y sella la clave. El
 * token nunca llega al cliente. Las mutaciones van con cuerpo JSON (CSRF).
 *
 * Ya no hay POST de «probar sin guardar»: guardar prueba antes de sellar, así que
 * el paso aparte solo duplicaba la petición (el servicio conserva /check por
 * compatibilidad).
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

const RUTA = '/api/v1/storage/external';

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function reenviar(res: Response | null): Promise<Response> {
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (res.status === 204) return new Response(null, { status: 204 });
  const cuerpo = await res.text();
  return new Response(cuerpo, {
    status: res.status >= 500 ? 502 : res.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function conCuerpo(ctx: Parameters<APIRoute>[0], ruta: string): Promise<Response> {
  let body: unknown;
  try {
    body = await ctx.request.json();
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  return reenviar(
    await driveFetch(ctx, ruta, {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    }),
  );
}

export const GET: APIRoute = async (ctx) => reenviar(await driveFetch(ctx, RUTA));
export const PUT: APIRoute = async (ctx) => conCuerpo(ctx, RUTA);
export const DELETE: APIRoute = async (ctx) =>
  reenviar(await driveFetch(ctx, RUTA, { method: 'DELETE' }));
