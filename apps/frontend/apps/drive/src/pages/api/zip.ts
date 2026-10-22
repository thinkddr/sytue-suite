/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — descarga de una CARPETA o SELECCIÓN como ZIP en streaming.
 *
 *   GET /api/zip?file_ids=a,b&folder_ids=c,d[&name=Docs]   Descarga un ZIP.
 *
 * El servicio Go recorre el árbol accesible (gateado por ítem: RLS + vivo) y escribe
 * el ZIP directo al cuerpo (memoria O(1), sin bufferizar). Aquí solo se reenvía ese
 * cuerpo en STREAMING (sin cargarlo en memoria) preservando el Content-Disposition
 * que ya compone el servicio. GET navegable (un <a download>): sin cuerpo → sin
 * CSRF. El access token de la sesión httpOnly va server-side (driveFetch); NUNCA
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

export const GET: APIRoute = async (ctx) => {
  const params = new URL(ctx.request.url).searchParams;
  const fileIds = params.get('file_ids')?.trim() ?? '';
  const folderIds = params.get('folder_ids')?.trim() ?? '';
  const name = params.get('name')?.trim() ?? '';
  if (!fileIds && !folderIds) return json({ error: 'empty_selection' }, 400);

  const qs = new URLSearchParams();
  if (fileIds) qs.set('file_ids', fileIds);
  if (folderIds) qs.set('folder_ids', folderIds);
  if (name) qs.set('name', name);

  const res = await driveFetch(ctx, `/api/v1/batch/zip?${qs.toString()}`);
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok || !res.body)
    return json({ error: 'zip_failed' }, res.status >= 500 ? 502 : res.status);

  // Reenvía el ZIP en streaming (sin bufferizarlo) con la disposición segura
  // (attachment + nosniff) y el filename* que ya compone el servicio Go.
  const headers = new Headers({
    'content-type': res.headers.get('content-type') ?? 'application/zip',
    'content-disposition':
      res.headers.get('content-disposition') ?? `attachment; filename="sytue-drive.zip"`,
    'x-content-type-options': 'nosniff',
    'cache-control': 'no-store',
  });
  return new Response(res.body, { status: 200, headers });
};
