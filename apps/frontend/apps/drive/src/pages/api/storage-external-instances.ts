/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/storage-external-instances → { items: [{ id, template_key, name, connected }] }
 * Las cuentas de Google Drive, OneDrive y Dropbox ya autorizadas para la
 * organización, para reutilizar una como almacén de Drive sin repetir el consentimiento.
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const res = await driveFetch(ctx, '/api/v1/storage/external/instances');
  if (!res) return new Response(JSON.stringify({ error: 'unauthenticated' }), { status: 401 });
  const cuerpo = await res.text();
  return new Response(cuerpo, {
    status: res.status >= 500 ? 502 : res.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};
