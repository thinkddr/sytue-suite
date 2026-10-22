/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — documentos.
 *
 *   GET    /api/documents                 Lista paginada (limit, cursor, q, kind) → {documents, nextCursor?}.
 *   GET    /api/documents?id=<id>         Metadatos de un documento concreto.
 *   DELETE /api/documents?id=<id>         Elimina un documento (el servicio Go valida el rol).
 *
 * Llama al servicio Go propio (docs.sytue.eu) via DocsClient (@sytue/api-clients)
 * con el access token de la sesion httpOnly. El token NUNCA llega al cliente.
 * Implementacion clean-room: SIN motores de terceros.
 */
import type { APIRoute } from 'astro';
import { DocsClient, bearer, HttpError, type DocKind } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl } from '../../lib/auth';
import { proxyDocs, json, requireId } from '../../lib/docsProxy';

export const prerender = false;

function isKind(value: string | null): value is DocKind {
  return value === 'document' || value === 'spreadsheet' || value === 'presentation';
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const client = new DocsClient({
    baseUrl: docsBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });

  const params = new URL(ctx.request.url).searchParams;
  const id = params.get('id');

  try {
    if (id) {
      // El ROL viaja al cliente junto al documento: sin él, el editor deja
      // teclear a quien solo puede leer y el fallo aparece al guardar.
      const { document, role, editRequested } = await client.get(id);
      // `edit_requested` como lo lee `useDocument`: la franja dice «Petición enviada».
      return json({ document, role, edit_requested: editRequested });
    }
    const kindParam = params.get('kind');
    const q = params.get('q')?.trim();
    const cursor = params.get('cursor');
    // Página acotada al rango del servicio (1..200); 100 por defecto.
    const parsed = Number.parseInt(params.get('limit') ?? '', 10);
    const limit = Number.isNaN(parsed) ? 100 : Math.min(Math.max(parsed, 1), 200);
    const page = await client.list({
      limit,
      ...(isKind(kindParam) ? { kind: kindParam } : {}),
      ...(q ? { q } : {}),
      ...(cursor ? { cursor } : {}),
      // ?starred=1 — el filtro «Destacados» lo resuelve el servicio; con keyset,
      // filtrar la página traída mentiría en cuanto hubiera más de una.
      ...(params.get('starred') === '1' ? { starred: true } : {}),
    });
    return json(page);
  } catch (e) {
    return json({ error: 'load_failed' }, e instanceof HttpError && e.status === 401 ? 401 : 502);
  }
};

export const DELETE: APIRoute = (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, `/documents/${encodeURIComponent(id)}`, { method: 'DELETE' });
};
