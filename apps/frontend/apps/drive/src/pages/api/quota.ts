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
import { driveQuotaFromResponse, type DriveQuotaResponse } from '@sytue/api-clients/drive';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const res = await driveFetch(ctx, '/api/v1/quota');
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'quota_failed' }, res.status >= 500 ? 502 : res.status);
  return json(driveQuotaFromResponse((await res.json()) as DriveQuotaResponse));
};
