/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — destacar/quitar de destacados.
 *
 *   POST /api/star  body {id, starred}  →  PUT /documents/{id}/star body {starred}
 *
 * Por POST y no por PUT del navegador, igual que /api/rename: el proxy hace el
 * verbo real. El estado es POR USUARIO (columna starred_by) y vive en el
 * SERVIDOR: destacado en el portátil se ve destacado en el móvil.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, json, esUuid } from '../../lib/docsProxy';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  let payload: { id?: unknown; starred?: unknown };
  try {
    payload = (await ctx.request.json()) as { id?: unknown; starred?: unknown };
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const id = typeof payload.id === 'string' ? payload.id : '';
  if (!id || !esUuid(id)) return json({ error: 'missing_id' }, 400);
  if (typeof payload.starred !== 'boolean') return json({ error: 'missing_starred' }, 400);
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/star`, {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ starred: payload.starred }),
  });
};
