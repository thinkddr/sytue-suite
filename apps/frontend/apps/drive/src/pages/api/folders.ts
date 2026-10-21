/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — carpetas.
 *
 *   POST /api/folders  { path: '/ruta/nueva-carpeta' }
 *
 * Crea una carpeta en el servicio Go propio (drive.sytue.eu) via DriveClient con
 * el access token de la sesion httpOnly. El token NUNCA llega al cliente.
 */
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { driveClientFor } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  let body: { path?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const path = typeof body.path === 'string' ? body.path.trim() : '';
  if (!path) return json({ error: 'missing_path' }, 400);

  try {
    const resource = await client.createFolder(path);
    return json({ resource }, 201);
  } catch (e) {
    return json({ error: 'folder_failed' }, upstreamStatus(e));
  }
};
