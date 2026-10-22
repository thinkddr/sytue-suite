/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — la galería de plantillas de «Crear nuevo».
 *
 *   GET /api/templates  →  GET /templates?locale=<idioma de la sesión>
 *
 * El idioma lo pone el servidor (el negociado por el middleware), no el
 * navegador: nombre y descripción llegan ya traducidos. Crear desde una de ellas
 * es POST /api/create con `template`.
 */
import type { APIRoute } from 'astro';
import { proxyDocs } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = (ctx) =>
  proxyDocs(ctx, `/templates?locale=${encodeURIComponent(ctx.locals.locale)}`);
