/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — «Detalles» de un fichero o una carpeta.
 *
 *   GET /api/details?id=<uuid>&kind=file|folder   → { details }
 *
 * Reúne en una respuesta lo que el servicio Go da en dos (el metadato y las
 * comparticiones) y pone nombre a las personas con el directorio de la organización.
 * La lógica vive en `lib/detalles.ts`; aquí solo se le da la sesión. El access token
 * viaja server-side (driveFetch): nunca llega al navegador.
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';
import { resolverPersonas } from '../../lib/directorio';
import { reunirDetalles } from '../../lib/detalles';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const params = new URL(ctx.request.url).searchParams;
  const id = params.get('id')?.trim();
  if (!id) return json({ error: 'missing_id' }, 400);
  const kind = params.get('kind') === 'folder' ? 'folder' : 'file';

  const { status, body } = await reunirDetalles(id, kind, {
    pedir: (ruta) => driveFetch(ctx, ruta),
    resolver: (subs) => resolverPersonas(ctx, subs),
  });
  return json(body, status);
};
