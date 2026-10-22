/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — copia server-side («Hacer una copia»).
 *
 *   POST /api/copy  { id, kind: 'file' | 'folder' }
 *
 * El servicio Go crea una fila de metadatos NUEVA que referencia el MISMO blob que
 * el original (sin descargar+resubir ni duplicar bytes; O(1) en bytes), «Copia de X»
 * única en el destino. La carpeta se copia recursivamente. La copia va a la misma
 * carpeta del origen (por defecto), así que aquí no se envía destino.
 *
 * POST con cuerpo JSON (no dispara el checkOrigin CSRF de Astro tras el proxy). El
 * access token de la sesión httpOnly va server-side (driveFetch); NUNCA llega al
 * cliente. Clean-room: SIN motores de terceros.
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  let body: { id?: unknown; kind?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const id = typeof body.id === 'string' ? body.id.trim() : '';
  const kind = body.kind === 'folder' ? 'folders' : body.kind === 'file' ? 'files' : '';
  if (!id) return json({ error: 'missing_id' }, 400);
  if (!kind) return json({ error: 'invalid_kind' }, 400);

  // Copia en la misma carpeta (sin cuerpo: el servicio Go usa la carpeta del origen).
  const res = await driveFetch(ctx, `/api/v1/${kind}/${encodeURIComponent(id)}/copy`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: '{}',
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'copy_failed' }, res.status >= 500 ? 502 : res.status);
  // Reenvía el recurso creado tal cual (el cliente recarga la carpeta de todos modos).
  const text = await res.text();
  return new Response(text || '{"ok":true}', {
    status: 201,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};
