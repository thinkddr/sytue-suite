/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — los bloques reutilizables que la cinta ofrece para insertar.
 *
 *   GET /api/blocks?kind=              →  GET /blocks?kind=&locale=        los de Sytue y los de la organización
 *   GET /api/blocks?key=[&version=]    →  GET /blocks/{key}?locale=…       el contenido de uno, ya con la marca
 *
 * El idioma lo pone el servidor (el negociado por el middleware), como en la galería.
 */
import type { APIRoute } from 'astro';
import { json, proxyDocs } from '../../lib/docsProxy';

export const prerender = false;

const KINDS = new Set(['document', 'spreadsheet', 'presentation']);

export const GET: APIRoute = (ctx) => {
  const q = ctx.url.searchParams;
  const locale = encodeURIComponent(ctx.locals.locale);
  const key = q.get('key');
  if (key) {
    const version = q.get('version');
    return proxyDocs(
      ctx,
      `/blocks/${encodeURIComponent(key)}?locale=${locale}` +
        (version ? `&version=${encodeURIComponent(version)}` : ''),
    );
  }
  const kind = q.get('kind') ?? '';
  if (!KINDS.has(kind)) return json({ error: 'invalid_kind' }, 400);
  return proxyDocs(ctx, `/blocks?kind=${kind}&locale=${locale}`);
};
