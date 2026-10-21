/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — guardar los comentarios de un fichero importado.
 *
 *   POST /api/comments-import?id=  →  POST /documents/{id}/comments/import
 *
 * El cuerpo es `{ comments }` tal como los devolvió `/api/import?comments=1`; la respuesta
 * dice qué anclas quedaron guardadas (`anchored`).
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json } from '../../lib/docsProxy';
import { rutaImportarComentarios } from '../../lib/comentarios-importados';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const body = await ctx.request.text();
  return proxyDocs(ctx, rutaImportarComentarios(id), {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body,
  });
};
