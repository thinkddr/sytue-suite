/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — canal de colaboracion en tiempo real.
 *
 *   GET /api/collab?id=<id>   Devuelve la URL WebSocket (CRDT/OT) del documento.
 *
 * El servicio Go (docs.sytue.eu) exige el access token en la query porque el
 * WebSocket no admite cabeceras personalizadas (lo valida via JWKS de Sytue ID).
 * El BFF resuelve la sesion httpOnly y compone la URL server-side, de modo que el
 * cliente recibe solo la URL de conexion, ya lista, sin manejar la sesion.
 */
import type { APIRoute } from 'astro';
import { DocsClient, bearer, HttpError } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl } from '../../lib/auth';
import { requireId } from '../../lib/docsProxy';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);

  const client = new DocsClient({
    baseUrl: docsBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });

  try {
    const url = await client.collabUrl(id);
    return json({ url });
  } catch (e) {
    return json({ error: 'collab_failed' }, e instanceof HttpError && e.status === 401 ? 401 : 502);
  }
};
