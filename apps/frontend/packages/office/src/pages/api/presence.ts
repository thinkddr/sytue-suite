/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Docs — presencia por heartbeat (BLUEPRINT.md §4.4).
 *
 *   PUT /api/presence?id=<id>   →  PUT /documents/{id}/presence   { members: [{ conn, sub }] }
 *
 * La presencia la guarda y la AUTORIZA el servicio (rol de lector como mínimo en ese
 * documento, `services/docs/internal/http/collab.go`). Hasta el 2026-10-04 este fichero
 * llevaba su propio registro en memoria, sin mirar el documento: cualquier sesión, de
 * cualquier organización, podía preguntar por un id ajeno y recibir los `sub` de quienes lo
 * tenían abierto, y anunciarse en él. Nació así porque la ruta del servicio aún no estaba
 * desplegada; lo está, y un registro paralelo aquí es una segunda puerta sin cerradura.
 *
 * Sin acceso se contesta 404, exista o no el documento: quien no puede verlo no tiene por
 * qué saber si existe.
 */
import type { APIRoute } from 'astro';
import { json, proxyDocs, requireId } from '../../lib/docsProxy';

export const prerender = false;

/** Cota del identificador de conexión del cliente (UUID = 36 chars), la del servicio. */
const MAX_CONN = 64;

export const PUT: APIRoute = async (ctx) => {
  const id = requireId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const { conn } = (await ctx.request.json().catch(() => ({}))) as { conn?: unknown };
  if (typeof conn !== 'string' || conn === '' || conn.length > MAX_CONN) {
    return json({ error: 'invalid_conn' }, 400);
  }
  // Solo `conn` viaja: quién es lo pone el servicio a partir del token de la sesión.
  const res = await proxyDocs(ctx, `/documents/${encodeURIComponent(id)}/presence`, {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ conn }),
  });
  return res.status === 403 ? json({ error: 'not_found' }, 404) : res;
};
