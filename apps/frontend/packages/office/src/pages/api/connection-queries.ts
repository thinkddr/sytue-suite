/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /api/connection-queries  (application/json)
 *
 * CONSULTAS GUARDADAS de un origen de datos externo (Power Query para Sheets).
 * SOLO proxy a `/api/v1/connectors/{connId}/queries` del servicio Go con la
 * sesión httpOnly como Authorization: Bearer (memoria «backend siempre Go»: el
 * saneado de la spec contra el tipo de la conexión, el aislamiento por tenant
 * con RLS y la comprobación de que la conexión es del tenant viven allí):
 *
 *   GET    /api/connection-queries?connection=<id>            → { queries: [...] }
 *   POST   /api/connection-queries?connection=<id>            → { query: {...} }
 *   PATCH  /api/connection-queries?connection=<id>&query=<id> → 204
 *   DELETE /api/connection-queries?connection=<id>&query=<id> → 204
 *
 * Va en su propio fichero y no en connections.ts porque son otro recurso y los
 * verbos ya estaban ocupados por la conexión. Cada id se interpola con
 * encodeURIComponent antes de llegar al proxy (frontera de confianza). El
 * content-type application/json evita el checkOrigin (CSRF) de Astro.
 *
 * Lo que hace REAL esto: sin ello, la spec de una consulta solo vivía dentro del
 * libro donde se creó (ExternalTable.spec del doc), así que reutilizarla en otra
 * hoja obligaba a reescribirla entera —ruta, parámetros y el `root_path`, que es
 * justo el que se encuentra a base de probar—.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, json } from '../../lib/docsProxy';

export const prerender = false;

/** Una consulta es nombre + spec: pequeña. Se rechaza antes de leer el cuerpo. */
const MAX_BYTES = 64 * 1024;

const JSON_HEADERS = { 'content-type': 'application/json' } as const;

/** Base `/connectors/<connId>/queries`; null si falta el id de la conexión. */
function base(url: string): string | null {
  const conn = new URL(url).searchParams.get('connection');
  return conn ? `/connectors/${encodeURIComponent(conn)}/queries` : null;
}

/** Ruta de UNA consulta; null si falta cualquiera de los dos ids. */
function one(url: string): string | null {
  const raiz = base(url);
  const query = new URL(url).searchParams.get('query');
  return raiz && query ? `${raiz}/${encodeURIComponent(query)}` : null;
}

/** Lee el cuerpo textual respetando el tope; null si excede. */
async function readBody(request: Request): Promise<string | null> {
  const len = Number(request.headers.get('content-length') ?? '0');
  if (Number.isFinite(len) && len > MAX_BYTES) return null;
  const body = await request.text();
  return body.length > MAX_BYTES ? null : body;
}

export const GET: APIRoute = (ctx) => {
  const path = base(ctx.request.url);
  return path ? proxyDocs(ctx, path) : json({ error: 'missing_connection' }, 400);
};

export const POST: APIRoute = async (ctx) => {
  const path = base(ctx.request.url);
  if (!path) return json({ error: 'missing_connection' }, 400);
  const body = await readBody(ctx.request);
  if (body === null) return json({ error: 'bad_request' }, 413);
  return proxyDocs(ctx, path, { method: 'POST', headers: JSON_HEADERS, body });
};

export const PATCH: APIRoute = async (ctx) => {
  const path = one(ctx.request.url);
  if (!path) return json({ error: 'missing_id' }, 400);
  const body = await readBody(ctx.request);
  if (body === null) return json({ error: 'bad_request' }, 413);
  return proxyDocs(ctx, path, { method: 'PATCH', headers: JSON_HEADERS, body });
};

export const DELETE: APIRoute = (ctx) => {
  const path = one(ctx.request.url);
  return path ? proxyDocs(ctx, path, { method: 'DELETE' }) : json({ error: 'missing_id' }, 400);
};
