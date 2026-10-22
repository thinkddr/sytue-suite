/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — operaciones CRDT (BLUEPRINT.md §4.2).
 *
 *   POST /api/ops?id=<id>                    →  POST /documents/{id}/ops (cuerpo tal cual)
 *   GET  /api/ops?id=<id>&since=<n>&limit=<n> →  GET  /documents/{id}/ops?since=&limit=
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json } from '../../lib/docsProxy';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const body = await ctx.request.text();
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/ops`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body,
  });
};

export const GET: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const params = new URL(ctx.request.url).searchParams;
  // Frontera de confianza: since/limit se validan numéricos (§4.1).
  const since = Number(params.get('since') ?? '0');
  const limit = Number(params.get('limit') ?? '200');
  if (!Number.isFinite(since) || !Number.isFinite(limit)) {
    return json({ error: 'bad_request' }, 400);
  }
  return proxyDocs(
    ctx,
    `/documents/${encodeURIComponent(id)}/ops?since=${Math.max(0, Math.floor(since))}&limit=${Math.min(500, Math.max(1, Math.floor(limit)))}`,
  );
};
