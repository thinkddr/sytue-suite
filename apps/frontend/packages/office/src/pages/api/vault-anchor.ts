/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — anclar un documento a su fichero del Drive de la bóveda.
 *
 *   POST /api/vault-anchor  body {id, source_ref, expect}
 *     →  PUT /documents/{id}/vault-anchor  body {source_ref, expect}
 *
 * Con el Drive de la persona en la bóveda (ADR 0086), el fichero del documento lo
 * escribe el NAVEGADOR, cifrado; al servicio solo le llega el ancla
 * («vault:<id del fichero>»), nunca el contenido. Por POST y no por PUT del
 * navegador, igual que /api/star: el proxy hace el verbo real. Quién puede anclar
 * (solo el dueño, solo sin ancla o desde otra de la bóveda) lo decide el servicio.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, json, esUuid } from '../../lib/docsProxy';
import { esAnclaDeBoveda } from '../../editor/state/anclaDrive';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  let payload: { id?: unknown; source_ref?: unknown; expect?: unknown };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const id = typeof payload.id === 'string' ? payload.id : '';
  if (!id || !esUuid(id)) return json({ error: 'missing_id' }, 400);
  const ancla = typeof payload.source_ref === 'string' ? payload.source_ref : '';
  if (!esAnclaDeBoveda(ancla)) return json({ error: 'bad_anchor' }, 400);
  const antes = typeof payload.expect === 'string' ? payload.expect : '';
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/vault-anchor`, {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ source_ref: ancla, expect: antes }),
  });
};
