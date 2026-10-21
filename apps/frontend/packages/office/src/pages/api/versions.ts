/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — versiones (BLUEPRINT.md §4.2).
 *
 *   GET  /api/versions?id=&limit=              →  GET  /documents/{id}/versions?limit=
 *   POST /api/versions?id=                     →  POST /documents/{id}/versions
 *   POST /api/versions?id=&restore=<versionId> →  POST /documents/{id}/versions/{versionId}/restore
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json, esUuid } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  // Frontera de confianza: limit numérico (§4.1).
  const limit = Number(new URL(ctx.request.url).searchParams.get('limit') ?? '50');
  if (!Number.isFinite(limit)) return json({ error: 'bad_request' }, 400);
  return proxyDocs(
    ctx,
    `/documents/${encodeURIComponent(id)}/versions?limit=${Math.min(200, Math.max(1, Math.floor(limit)))}`,
  );
};

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const restore = new URL(ctx.request.url).searchParams.get('restore');
  if (restore) {
    if (!esUuid(restore)) return json({ error: 'bad_request' }, 400);
    // El cuerpo lleva, si hace falta, la contraseña de protección de la hoja (`proteccion`).
    const cuerpo = await ctx.request.text();
    return proxyDocs(
      ctx,
      `/documents/${encodeURIComponent(id)}/versions/${encodeURIComponent(restore)}/restore`,
      {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: cuerpo || '{}',
      },
    );
  }
  const body = await ctx.request.text();
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/versions`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: body || '{}',
  });
};
