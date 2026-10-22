/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/dataquery  (application/json)
 *
 * Conector de datos externos (Power Query para Sheets): EJECUTAR/REFRESCAR una
 * consulta. SOLO proxy — reenvía el cuerpo VERBATIM a `POST /api/v1/connectors/
 * execute` del servicio Go con la sesión httpOnly como Authorization: Bearer
 * (memoria «backend siempre Go»; toda la lógica —descifrado de credenciales,
 * barrera SSRF, guard de SELECT, límites y rate-limit por tenant— vive allí).
 *
 * Cuerpo (modo inline, el que usa el editor): { connection_id, query } donde
 * `query` es la QuerySpec (rest | postgres). Respuesta 200: { columns, rows }.
 * El content-type application/json evita el checkOrigin (CSRF) de Astro, que solo
 * bloquea los tipos «de formulario». Las credenciales NUNCA viajan aquí: se
 * resuelven cifradas en el backend a partir de `connection_id`.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, json } from '../../lib/docsProxy';

export const prerender = false;

/** El cuerpo de execute es pequeño (id + consulta); holgura amplia por si lleva
 *  un body JSON en la consulta REST. Se rechaza antes de bufferizar. */
const MAX_BYTES = 256 * 1024;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const len = Number(ctx.request.headers.get('content-length') ?? '0');
  if (Number.isFinite(len) && len > MAX_BYTES) return json({ error: 'bad_request' }, 413);
  const body = await ctx.request.text();
  return proxyDocs(ctx, '/connectors/execute', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body,
  });
};
