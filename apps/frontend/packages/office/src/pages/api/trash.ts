/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — la papelera (components/DocTrash.tsx).
 *
 *   GET  /api/trash[?kind=]                      →  GET    /trash[?kind=]
 *   POST /api/trash {action:'restore', id}       →  POST   /trash/{id}/restore
 *   POST /api/trash {action:'purge', id}         →  DELETE /trash/{id}
 *   POST /api/trash {action:'empty', kind?, after?} → DELETE /trash[?kind=&after=]
 *
 * Todo por POST desde el navegador, como /api/star: el proxy hace el verbo real.
 * Restaurar y eliminar llevan también el fichero del documento en Sytue Drive; eso
 * lo hace el servicio.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, json, esUuid } from '../../lib/docsProxy';
import { esKind } from '../../lib/producto';

export const prerender = false;

function consulta(kind: unknown, after?: unknown): string {
  const q = new URLSearchParams();
  if (typeof kind === 'string' && esKind(kind)) q.set('kind', kind);
  if (typeof after === 'string' && after) q.set('after', after);
  const s = q.toString();
  return s ? `?${s}` : '';
}

export const GET: APIRoute = (ctx) =>
  proxyDocs(ctx, `/trash${consulta(ctx.url.searchParams.get('kind'))}`);

export const POST: APIRoute = async (ctx) => {
  let payload: { action?: unknown; id?: unknown; kind?: unknown; after?: unknown };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const id = typeof payload.id === 'string' && esUuid(payload.id) ? payload.id : '';
  switch (payload.action) {
    case 'restore':
      if (!id) return json({ error: 'missing_id' }, 400);
      return proxyDocs(ctx, `/trash/${encodeURIComponent(id)}/restore`, { method: 'POST' });
    case 'purge':
      if (!id) return json({ error: 'missing_id' }, 400);
      return proxyDocs(ctx, `/trash/${encodeURIComponent(id)}`, { method: 'DELETE' });
    case 'empty':
      return proxyDocs(ctx, `/trash${consulta(payload.kind, payload.after)}`, { method: 'DELETE' });
    default:
      return json({ error: 'bad_action' }, 400);
  }
};
