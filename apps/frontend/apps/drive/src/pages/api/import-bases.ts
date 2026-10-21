/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — EXPORTAR UNA VISTA DE SYTUE BASES a la carpeta abierta.
 *
 *   POST /api/import-bases  { path, token }  →  201 { name }
 *
 * `token` es el de una conexión de solo lectura que el propietario de la base concedió
 * para Sytue Drive. Va en el cuerpo, se manda UNA vez al servicio (que lee de Bases y
 * escribe el CSV con su procedencia) y no se guarda en ningún sitio.
 */
import type { APIRoute } from 'astro';
import { HttpError } from '@sytue/api-clients';
import { driveClientFor } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  let p: { path?: unknown; token?: unknown };
  try {
    p = (await ctx.request.json()) as typeof p;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  if (typeof p.path !== 'string' || typeof p.token !== 'string' || p.token.length > 200) {
    return json({ error: 'invalid_body' }, 400);
  }
  try {
    const f = await client.importBasesView(p.path, p.token.trim());
    return json({ name: f.name }, 201);
  } catch (e) {
    // Solo dos motivos llegan al navegador con nombre: la conexión no vale, o no cabe.
    if (e instanceof HttpError && e.code === 'bases_connection_invalid')
      return json({ error: 'bases_connection_invalid', message: 'bases_connection_invalid' }, 422);
    if (e instanceof HttpError && e.status === 507)
      return json({ error: 'quota_exceeded', message: 'quota_exceeded' }, 507);
    return json({ error: 'import_failed' }, 502);
  }
};
