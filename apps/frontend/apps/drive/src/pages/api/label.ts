/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — ETIQUETA DE SENSIBILIDAD de un fichero.
 *
 *   GET  /api/label                          Taxonomía de la organización
 *   POST /api/label  { fileId, labelId }     Clasifica el fichero
 *   POST /api/label  { fileId }              Quita la clasificación
 *
 * La taxonomía la define un administrador en Sytue Admin; aquí solo se APLICA. El
 * servicio rechaza un id que no esté en la taxonomía de la organización que pregunta,
 * así que no hace falta revalidarlo aquí (y hacerlo duplicaría la fuente de verdad).
 *
 * Quitar va por POST sin `labelId` y no por DELETE: el checkOrigin (CSRF) de Astro
 * rechaza DELETE tras el proxy, y es el mismo criterio que ya sigue el resto del BFF de
 * Drive. El servicio Go sí expone DELETE, que es lo correcto en su API.
 *
 * El access token de la sesión httpOnly viaja server-side (driveFetch): nunca llega al
 * navegador.
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

export const GET: APIRoute = async (ctx) => {
  const res = await driveFetch(ctx, '/api/v1/sensitivity-labels');
  if (!res) return json({ error: 'unauthenticated' }, 401);
  // Sin taxonomía definida NO es un error: la organización todavía no clasifica nada, y
  // la UI debe poder decirlo en vez de enseñar un fallo.
  if (!res.ok) return json({ labels: [] });
  const text = await res.text();
  return new Response(text || '{"labels":[]}', {
    status: 200,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};

export const POST: APIRoute = async (ctx) => {
  let body: { fileId?: unknown; labelId?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const fileId = typeof body.fileId === 'string' ? body.fileId.trim() : '';
  if (!fileId) return json({ error: 'missing_file' }, 400);
  const labelId = typeof body.labelId === 'string' ? body.labelId.trim() : '';

  const path = `/api/v1/files/${encodeURIComponent(fileId)}/label`;
  const res = labelId
    ? await driveFetch(ctx, path, {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ label_id: labelId }),
      })
    : await driveFetch(ctx, path, { method: 'DELETE' });

  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'label_failed' }, res.status >= 500 ? 502 : res.status);
  const text = await res.text();
  return new Response(text || '{"ok":true}', {
    status: 200,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};
