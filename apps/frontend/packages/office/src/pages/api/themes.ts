/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — los temas comunes y la marca de la organización.
 *
 *   GET /api/themes                →  GET /themes                    catálogo, marca vigente y versiones
 *   PUT /api/themes                →  PUT /brand                     una versión nueva de la marca (administradores)
 *   POST /api/themes?id=           →  POST /documents/{id}/theme     aplicar un tema a un documento (editores)
 *
 * Quién puede qué lo decide el servicio; aquí solo se reenvía.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = (ctx) => proxyDocs(ctx, '/themes');

export const PUT: APIRoute = async (ctx) =>
  proxyDocs(ctx, '/brand', {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: (await ctx.request.text()) || '{}',
  });

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/theme`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: (await ctx.request.text()) || '{}',
  });
};
