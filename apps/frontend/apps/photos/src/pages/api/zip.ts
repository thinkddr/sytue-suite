/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/zip?file_ids=a,b[&name=Fotos 2026-11-10] — la selección de la galería
 * como UN ZIP, armado por Drive (`/api/v1/batch/zip`) con los originales.
 *
 * POR QUÉ UN ZIP. Descargar foto a foto obligaba a goteo (el navegador bloquea
 * ráfagas de descargas) y a un tope de 25; un ZIP es una sola descarga, sin tope
 * práctico, y trae el original y no una exportación JPEG.
 *
 * POR QUÉ PASA POR AQUÍ: por lo mismo que `drive-subir.ts` — Drive vive en otro
 * host sin CORS y el token de la sesión no sale del servidor. Es un GET navegable
 * (un <a download>), sin cuerpo y sin efectos: no necesita CSRF.
 *
 * EL ZIP VA EN STREAMING: Drive lo escribe según recorre los ficheros y aquí se
 * reenvía el cuerpo sin leerlo; bufferizar cientos de fotos tumbaría la réplica.
 * El permiso lo decide Drive por fichero (RLS): pedir un id ajeno no filtra nada.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from '../../lib/bff.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
// Por el Service del clúster, como en drive-subir.ts (ahí está el porqué).
const drive = (
  env.SYTUE_DRIVE_INTERNAL_URL ??
  env.SYTUE_DRIVE_API_URL ??
  'http://drive:8082'
).replace(/\/$/, '');

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const sesion = await bff.getSession(toBffContext(ctx));
  if (!sesion) return json({ error: 'unauthenticated' }, 401);

  const params = new URL(ctx.request.url).searchParams;
  const ficheros = params.get('file_ids')?.trim() ?? '';
  const nombre = params.get('name')?.trim() ?? '';
  if (!ficheros) return json({ error: 'empty_selection' }, 400);

  // Solo ficheros: la galería no descarga carpetas de Drive.
  const qs = new URLSearchParams({ file_ids: ficheros, name: nombre || 'sytue-fotos' });
  const res = await fetch(`${drive}/api/v1/batch/zip?${qs}`, {
    headers: { authorization: `Bearer ${sesion.tokens.accessToken}` },
  });
  if (!res.ok || !res.body) {
    return json({ error: 'zip_failed' }, res.status >= 500 || res.ok ? 502 : res.status);
  }

  // Se conserva la disposición que compone Drive (attachment + filename* RFC 5987).
  const headers = new Headers({
    'content-type': res.headers.get('content-type') ?? 'application/zip',
    'content-disposition':
      res.headers.get('content-disposition') ?? `attachment; filename="sytue-fotos.zip"`,
    'x-content-type-options': 'nosniff',
    'cache-control': 'no-store',
  });
  return new Response(res.body, { status: 200, headers });
};
