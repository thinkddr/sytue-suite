/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/sender-logo?d=vodafone.es → el logo de la web de ese dominio, para el
 * avatar de la fila. Proxy BFF al servicio Go (GET /api/v1/sender-logo), que es quien
 * lo busca, lo reduce a PNG y lo guarda por dominio: la misma pieza que usa la app
 * nativa. 404 si no hay uno aprovechable: el `<img>` falla y vuelven las iniciales.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet } from '../../lib/mailrest.js';
import { dominioRegistrable } from '../../lib/dominioRemitente.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return vacio(401, 'no-store');

  // Lo que manda el navegador no se da por bueno: el servicio lo vuelve a validar.
  const pedido = ctx.url.searchParams.get('d') ?? '';
  const dominio = dominioRegistrable(pedido);
  if (!dominio || dominio !== pedido.trim().toLowerCase()) return vacio(400, 'no-store');

  let res: Response;
  try {
    res = await mailGet(
      session.accessToken,
      `/api/v1/sender-logo?d=${encodeURIComponent(dominio)}`,
    );
  } catch {
    return vacio(502, 'no-store');
  }
  if (res.status === 404) return vacio(404, 'private, max-age=86400');
  if (!res.ok || res.headers.get('content-type') !== 'image/png') {
    void res.body?.cancel();
    return vacio(502, 'no-store');
  }
  return new Response(await res.arrayBuffer(), {
    headers: {
      'content-type': 'image/png',
      'cache-control': 'private, max-age=604800',
      'x-content-type-options': 'nosniff',
      'content-security-policy': "default-src 'none'; sandbox",
    },
  });
};

function vacio(status: number, cache: string): Response {
  return new Response(null, { status, headers: { 'cache-control': cache } });
}
