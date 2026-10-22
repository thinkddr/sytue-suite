/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — comentarios (BLUEPRINT.md §4.2).
 *
 *   GET    /api/comments?id=&resolved=        →  GET    /documents/{id}/comments[?resolved=true]
 *   POST   /api/comments?id=                  →  POST   /documents/{id}/comments
 *   PUT    /api/comments?id=&commentId=       →  PUT    /documents/{id}/comments/{commentId}/resolve
 *   DELETE /api/comments?id=&commentId=       →  DELETE /documents/{id}/comments/{commentId}
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json, esUuid } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const resolved = new URL(ctx.request.url).searchParams.get('resolved');
  const suffix = resolved === 'true' ? '?resolved=true' : '';
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/comments${suffix}`);
};

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const body = await ctx.request.text();
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/comments`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body,
  });
};

export const PUT: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  const commentId = new URL(ctx.request.url).searchParams.get('commentId');
  // El comentario es un UUID: `..` pasaría encodeURIComponent y fetch lo normalizaría.
  if (!id || !commentId || !esUuid(commentId)) return json({ error: 'missing_id' }, 400);
  const body = await ctx.request.text();
  return proxyDocs(
    ctx,
    `/documents/${encodeURIComponent(id)}/comments/${encodeURIComponent(commentId)}/resolve`,
    { method: 'PUT', headers: { 'content-type': 'application/json' }, body },
  );
};

export const DELETE: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  const commentId = new URL(ctx.request.url).searchParams.get('commentId');
  // El comentario es un UUID: `..` pasaría encodeURIComponent y fetch lo normalizaría.
  if (!id || !commentId || !esUuid(commentId)) return json({ error: 'missing_id' }, 400);
  return proxyDocs(
    ctx,
    `/documents/${encodeURIComponent(id)}/comments/${encodeURIComponent(commentId)}`,
    { method: 'DELETE' },
  );
};
