/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — uso de almacenamiento (cuota).
 *
 *   GET /api/quota   → { usedBytes, limitBytes, availableBytes }
 *
 * Proxy sobre el servicio Go propio (drive.sytue.eu) con el access token de la
 * sesión httpOnly. El DriveClient de @sytue/api-clients no cubre la cuota, así que
 * llamamos al endpoint autenticado directamente. El token NUNCA llega al cliente.
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

interface RawQuota {
  limit_bytes: number;
  used_bytes: number;
  available_bytes: number;
  /** Los ficheros nuevos van al bucket propio de la organización (no gastan cuota). */
  external_storage?: boolean;
}

export const GET: APIRoute = async (ctx) => {
  const res = await driveFetch(ctx, '/api/v1/quota');
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'quota_failed' }, res.status >= 500 ? 502 : res.status);
  const q = (await res.json()) as RawQuota;
  return json({
    usedBytes: q.used_bytes,
    limitBytes: q.limit_bytes,
    availableBytes: q.available_bytes,
    externalStorage: q.external_storage === true,
  });
};
