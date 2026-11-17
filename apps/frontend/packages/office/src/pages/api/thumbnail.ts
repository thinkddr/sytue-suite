/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — miniatura de la portada.
 *
 *   GET /api/thumbnail?id=<uuid>&v=<thumbnail_seq>  →  GET /documents/{id}/thumbnail?v=
 *
 * La pide un <img> de la tesela, así que no pasa por el `fetch` del editor: el BFF pone
 * el token de la sesión y devuelve los BYTES del PNG (proxyDocs los trataría como texto
 * y JSON). La caché es la que dice el servicio —inmutable con la secuencia vigente en la
 * URL, revalidar sin ella—, y siempre PRIVADA: es contenido del documento. Un «no» del
 * servicio (403, 404) llega tal cual y la tesela vuelve al glifo del tipo.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, docsBaseUrl } from '../../lib/auth';
import { json, requireId } from '../../lib/docsProxy';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  // Solo dígitos: es la versión de la URL, no un parámetro libre hacia el servicio.
  const v = new URL(ctx.request.url).searchParams.get('v') ?? '';
  const consulta = /^\d{1,19}$/.test(v) ? `?v=${v}` : '';

  let upstream: Response;
  try {
    upstream = await fetch(
      `${docsBaseUrl()}/api/v1/documents/${encodeURIComponent(id)}/thumbnail${consulta}`,
      { headers: { authorization: `Bearer ${session.tokens.accessToken}` } },
    );
  } catch {
    return json({ error: 'upstream_unreachable' }, 502);
  }
  if (!upstream.ok || upstream.headers.get('content-type') !== 'image/png') {
    return json({ error: 'no_thumbnail' }, upstream.ok ? 502 : upstream.status);
  }
  const cache = upstream.headers.get('cache-control') ?? '';
  return new Response(await upstream.arrayBuffer(), {
    status: 200,
    headers: {
      'content-type': 'image/png',
      'x-content-type-options': 'nosniff',
      'content-security-policy': "default-src 'none'; sandbox",
      'cache-control': cache.includes('immutable')
        ? 'private, max-age=31536000, immutable'
        : 'private, no-cache',
    },
  });
};
