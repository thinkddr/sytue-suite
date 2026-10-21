/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — renombrar documento (BLUEPRINT.md §4.2).
 *
 *   POST /api/rename  body {id, title}  →  PATCH /documents/{id} body {title}
 */
import type { APIRoute } from 'astro';
import { proxyDocs, json, esUuid } from '../../lib/docsProxy';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  let payload: { id?: unknown; title?: unknown };
  try {
    payload = (await ctx.request.json()) as { id?: unknown; title?: unknown };
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const id = typeof payload.id === 'string' ? payload.id : '';
  const title = typeof payload.title === 'string' ? payload.title.trim() : '';
  if (!id || !esUuid(id)) return json({ error: 'missing_id' }, 400);
  if (!title) return json({ error: 'missing_title' }, 400);
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ title }),
  });
};
