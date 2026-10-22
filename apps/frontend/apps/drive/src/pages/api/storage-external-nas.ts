/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NAS del cliente por el túnel de Sytue DevX.
 *   POST /api/storage-external-nas → crea el túnel; devuelve { endpoint, command }
 *   GET  /api/storage-external-nas → estado { online, reachable, command, error }
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

async function reenviar(res: Response | null): Promise<Response> {
  if (!res) return new Response(JSON.stringify({ error: 'unauthenticated' }), { status: 401 });
  const cuerpo = await res.text();
  return new Response(cuerpo, {
    status: res.status >= 500 ? 502 : res.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) =>
  reenviar(await driveFetch(ctx, '/api/v1/storage/external/nas/status'));

export const POST: APIRoute = async (ctx) => {
  let body: unknown;
  try {
    body = await ctx.request.json();
  } catch {
    return new Response(JSON.stringify({ error: 'invalid_body' }), { status: 400 });
  }
  return reenviar(
    await driveFetch(ctx, '/api/v1/storage/external/nas', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    }),
  );
};
