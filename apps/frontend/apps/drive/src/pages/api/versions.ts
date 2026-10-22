/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — historial de versiones de un fichero.
 *
 *   GET  /api/versions?id=<fileId>[&cursor=<opaco>&limit=N]  → { versions, nextCursor? }
 *   POST /api/versions   { id, version }                      → restaura → { resource }
 *
 * Petición autenticada al servicio Go propio (drive.sytue.eu) con el access token
 * de la sesión httpOnly (driveFetch); el token NUNCA llega al cliente. La primera
 * página del listado encabeza con la versión ACTUAL (is_current). Restaurar es NO
 * destructivo (la actual se archiva) y solo lo permite un editor (el servicio
 * gatea; un viewer recibe 403). POST con JSON no dispara el checkOrigin CSRF de
 * Astro (como /api/folders); es una mutación con cuerpo.
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';
import { toVersion, type RawVersion } from '../../lib/versions';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const params = new URL(ctx.request.url).searchParams;
  const id = params.get('id')?.trim();
  if (!id) return json({ error: 'missing_id' }, 400);
  const cursor = params.get('cursor')?.trim() || '';
  const limit = params.get('limit')?.trim() || '';
  const qs = new URLSearchParams();
  if (cursor) qs.set('cursor', cursor);
  if (limit) qs.set('limit', limit);
  const suffix = qs.toString() ? `?${qs}` : '';

  const res = await driveFetch(ctx, `/api/v1/files/${encodeURIComponent(id)}/versions${suffix}`);
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'versions_failed' }, res.status >= 500 ? 502 : res.status);
  const body = (await res.json()) as { versions?: RawVersion[]; next_cursor?: string };
  const versions = (body.versions ?? []).map(toVersion);
  return json({ versions, ...(body.next_cursor ? { nextCursor: body.next_cursor } : {}) });
};

export const POST: APIRoute = async (ctx) => {
  let body: { id?: unknown; version?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const id = typeof body.id === 'string' ? body.id.trim() : '';
  const version =
    typeof body.version === 'number' ? body.version : Number.parseInt(String(body.version), 10);
  if (!id) return json({ error: 'missing_id' }, 400);
  if (!Number.isFinite(version) || version <= 0) return json({ error: 'invalid_version' }, 400);

  const res = await driveFetch(
    ctx,
    `/api/v1/files/${encodeURIComponent(id)}/versions/${version}/restore`,
    { method: 'POST', headers: { 'content-type': 'application/json' } },
  );
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'restore_failed' }, res.status >= 500 ? 502 : res.status);
  // El cliente solo recarga la carpeta tras restaurar; el fichero devuelto no se usa.
  return json({ ok: true }, 200);
};
