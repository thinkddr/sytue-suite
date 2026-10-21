/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — el techo de caducidad de los enlaces de la organización.
 *
 *   GET /api/link-policy   →  GET /link-policy    máximo de días (0 = sin techo) y si quien pregunta lo administra
 *   PUT /api/link-policy   →  PUT /link-policy    cambiarlo (owner o admin de la organización)
 *
 * Quién puede cambiarlo y qué valores valen lo decide el servicio; aquí solo se reenvía.
 */
import type { APIRoute } from 'astro';
import { proxyDocs } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = (ctx) => proxyDocs(ctx, '/link-policy');

export const PUT: APIRoute = async (ctx) =>
  proxyDocs(ctx, '/link-policy', {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: (await ctx.request.text()) || '{}',
  });
