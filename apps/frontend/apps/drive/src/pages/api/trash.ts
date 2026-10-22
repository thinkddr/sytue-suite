/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — papelera.
 *
 *   GET    /api/trash                       Lista los elementos borrados (recuperables).
 *   POST   /api/trash  { id, kind }         Restaura un elemento a su carpeta original.
 *   DELETE /api/trash?id=&kind=             Elimina un elemento DEFINITIVAMENTE (purga el blob).
 *
 * Proxy sobre el servicio Go propio (drive.sytue.eu) via DriveClient con el access
 * token de la sesion httpOnly. El token NUNCA llega al cliente.
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

function fail(e: unknown, fallback: string): Response {
  if (e instanceof HttpError && e.status >= 400 && e.status < 500)
    // El MOTIVO del servicio (p. ej. retención legal al purgar, o el nombre en
    // conflicto al restaurar) se propaga para que la UI diga por qué, en vez de un
    // «no se pudo» seco. Solo el mensaje de los 4xx del servicio.
    return json({ error: fallback, message: upstreamMessage(e) }, e.status);
  return json({ error: fallback }, 502);
}

/** Mensaje legible del sobre de error del servicio Go; '' si no se puede leer. */
function upstreamMessage(e: unknown): string {
  if (!(e instanceof HttpError) || !e.body) return '';
  try {
    const parsed = JSON.parse(e.body) as { error?: { message?: unknown } };
    const msg = parsed?.error?.message;
    return typeof msg === 'string' ? msg : '';
  } catch {
    return '';
  }
}

/** Normaliza el tipo de recurso: 'folder' o (por defecto) 'file'. */
function kindOf(v: unknown): 'file' | 'folder' {
  return v === 'folder' ? 'folder' : 'file';
}

export const GET: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);
  try {
    const items = await client.listTrash();
    return json({ items });
  } catch (e) {
    return fail(e, 'trash_failed');
  }
};

export const POST: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  let body: { id?: unknown; kind?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const id = typeof body.id === 'string' ? body.id.trim() : '';
  if (!id) return json({ error: 'missing_id' }, 400);
  try {
    await client.restore(id, kindOf(body.kind));
    return json({ ok: true });
  } catch (e) {
    return fail(e, 'restore_failed');
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const id = (url.searchParams.get('id') || '').trim();
  if (!id) return json({ error: 'missing_id' }, 400);
  try {
    await client.purge(id, kindOf(url.searchParams.get('kind')));
    return json({ ok: true });
  } catch (e) {
    return fail(e, 'purge_failed');
  }
};
