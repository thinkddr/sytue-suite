/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — operaciones en LOTE (multiselección).
 *
 *   POST /api/batch  { op: 'move', fileIds, folderIds, targetId? }  Mueve N a otra carpeta
 *   POST /api/batch  { op: 'trash', fileIds, folderIds }            Envía N a la papelera
 *
 * El servicio Go aplica el lote en UNA operación (WHERE id = ANY), gateado por ítem
 * (RLS: lo ajeno se omite). targetId vacío/ausente = raíz «Mi unidad». POST con
 * cuerpo JSON (no dispara el checkOrigin CSRF de Astro tras el proxy). El access
 * token de la sesión httpOnly va server-side (driveFetch); NUNCA llega al cliente.
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

/** Filtra un valor a una lista de strings no vacíos (ids); ignora basura. */
function ids(v: unknown): string[] {
  if (!Array.isArray(v)) return [];
  return v.filter((x): x is string => typeof x === 'string' && x.trim() !== '');
}

export const POST: APIRoute = async (ctx) => {
  let body: { op?: unknown; fileIds?: unknown; folderIds?: unknown; targetId?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const op = body.op === 'move' ? 'move' : body.op === 'trash' ? 'trash' : '';
  if (!op) return json({ error: 'invalid_op' }, 400);

  const fileIds = ids(body.fileIds);
  const folderIds = ids(body.folderIds);
  if (fileIds.length + folderIds.length === 0) return json({ error: 'empty_selection' }, 400);

  const payload: Record<string, unknown> = { file_ids: fileIds, folder_ids: folderIds };
  // Solo «mover» lleva destino; vacío/ausente = raíz (lo resuelve el servicio Go).
  if (op === 'move')
    payload.target_folder_id = typeof body.targetId === 'string' ? body.targetId : '';

  const res = await driveFetch(ctx, `/api/v1/batch/${op}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: `${op}_failed` }, res.status >= 500 ? 502 : res.status);
  // Reenvía el JSON de contadores del servicio (moved_*/trashed_*) tal cual.
  const text = await res.text();
  return new Response(text || '{"ok":true}', {
    status: 200,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};
