/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /api/connections  (application/json)
 *
 * Gestor de CONEXIONES del conector de datos externos (Power Query para Sheets).
 * SOLO proxy a `/api/v1/connectors` del servicio Go con la sesión httpOnly como
 * Authorization: Bearer (memoria «backend siempre Go»; validación, cifrado de
 * credenciales at-rest, aislamiento por tenant con RLS — todo en el backend):
 *
 *   GET     /api/connections           → listar          → { connections: [...] }
 *   POST    /api/connections           → crear           → { connection: {...} }
 *   PATCH   /api/connections?id=<id>   → actualizar      → { connection: {...} }
 *   DELETE  /api/connections?id=<id>   → borrar          → 204
 *
 * Las credenciales (api-key/token/contraseña) SOLO viajan en el POST de crear o en
 * el PATCH que las renueva (navegador→backend por HTTPS); se cifran at-rest y NUNCA
 * vuelven al cliente (el backend expone solo `has_credentials`). El PATCH es lo que
 * hace REAL el consejo «la credencial ha caducado, vuelve a introducirla»: sin él,
 * la única salida sería borrar la conexión y rehacerla (y perder las tablas que la
 * referencian). El content-type application/json evita el checkOrigin (CSRF) de Astro.
 */
import type { APIRoute } from 'astro';
import { proxyDocs, requireId, json } from '../../lib/docsProxy';

export const prerender = false;

/** Config + credenciales de una conexión son pequeñas: se rechaza antes de leer. */
const MAX_BYTES = 64 * 1024;

const JSON_HEADERS = { 'content-type': 'application/json' } as const;

export const GET: APIRoute = (ctx) => proxyDocs(ctx, '/connectors/');

export const POST: APIRoute = async (ctx) => {
  const body = await readBody(ctx.request);
  if (body === null) return json({ error: 'bad_request' }, 413);
  return proxyDocs(ctx, '/connectors/', { method: 'POST', headers: JSON_HEADERS, body });
};

export const PATCH: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const body = await readBody(ctx.request);
  if (body === null) return json({ error: 'bad_request' }, 413);
  return proxyDocs(ctx, `/connectors/${encodeURIComponent(id)}/`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body,
  });
};

export const DELETE: APIRoute = (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  return proxyDocs(ctx, `/connectors/${encodeURIComponent(id)}/`, { method: 'DELETE' });
};

/** Lee el cuerpo textual respetando el tope; null si excede (frontera de confianza). */
async function readBody(request: Request): Promise<string | null> {
  const len = Number(request.headers.get('content-length') ?? '0');
  if (Number.isFinite(len) && len > MAX_BYTES) return null;
  const body = await request.text();
  return body.length > MAX_BYTES ? null : body;
}
