/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Calcus — vistas de filtro PERSONALES (services/docs/internal/http/vistas_filtro.go).
 *
 *   GET  /api/filter-views?id=<id>   →  GET /documents/{id}/filter-views   {state}
 *   POST /api/filter-views?id=<id>   →  PUT /documents/{id}/filter-views   body {state}
 *
 * Por POST y no por PUT del navegador, igual que /api/star. No lleva `sub`: el servicio
 * usa el de la sesión, así que no hay manera de pedir las vistas de otra persona.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/filter-views`);
};

export const POST: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/filter-views`, {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: await ctx.request.text(),
  });
};
