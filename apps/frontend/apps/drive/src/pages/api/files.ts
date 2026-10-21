/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — recursos (archivos/carpetas).
 *
 *   GET    /api/files?path=/ruta      Lista el contenido de una carpeta con
 *          [&cursor=<opaco>&limit=N]  paginación keyset (carpetas completas en
 *                                     la primera página; ficheros por páginas).
 *                                     Devuelve nextCursor cuando hay más.
 *   POST   /api/files                 { path, name? | toPath? } Renombra (name)
 *                                     o mueve (toPath) un recurso. POST y no
 *                                     PATCH: el CSRF de Astro rechaza métodos
 *                                     con cuerpo distintos de POST tras el proxy.
 *   DELETE /api/files?path=/ruta      Envia un recurso a la papelera.
 *
 * Llama al servicio Go propio (drive.sytue.eu) via DriveClient (@sytue/api-clients)
 * con el access token de la sesion httpOnly. El token NUNCA llega al cliente.
 * Implementacion clean-room: SIN motores de terceros.
 */
import type { APIRoute } from 'astro';
import { upstreamStatus } from '@sytue/api-clients';
import { driveClientFor } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function fail(e: unknown, fallback: string): Response {
  return json({ error: fallback }, upstreamStatus(e)); // 4xx del servicio se preservan
}

export const GET: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const params = new URL(ctx.request.url).searchParams;
  const path = params.get('path') || '/';
  const cursor = params.get('cursor')?.trim() || '';
  const limit = Number.parseInt(params.get('limit') ?? '', 10);
  try {
    // El servicio Go acota el límite (≤500) y valida el cursor (opaco); aquí
    // solo se descartan valores sin sentido para no propagar basura.
    const page = await client.listPage(path, {
      ...(cursor ? { cursor } : {}),
      ...(Number.isFinite(limit) && limit > 0 ? { limit } : {}),
    });
    return json({ items: page.items, ...(page.nextCursor ? { nextCursor: page.nextCursor } : {}) });
  } catch (e) {
    return fail(e, 'list_failed');
  }
};

export const POST: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  let body: { path?: unknown; name?: unknown; toPath?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const path = typeof body.path === 'string' ? body.path.trim() : '';
  const name = typeof body.name === 'string' ? body.name.trim() : '';
  const toPath = typeof body.toPath === 'string' ? body.toPath.trim() : '';
  if (!path) return json({ error: 'missing_path' }, 400);
  if (!name && !toPath) return json({ error: 'missing_target' }, 400);
  if (name.includes('/')) return json({ error: 'invalid_name' }, 400);
  // Mover una carpeta dentro de sí misma (o de un descendiente) crearía un ciclo.
  if (toPath === path || toPath.startsWith(`${path}/`))
    return json({ error: 'invalid_target' }, 400);

  const segs = path.split('/').filter(Boolean);
  const base = segs[segs.length - 1];
  if (!base) return json({ error: 'invalid_path' }, 400); // la raíz no se renombra/mueve

  // Renombrar: mismo padre, nuevo nombre. Mover: misma base, nueva carpeta.
  // DriveClient.move() resuelve ambos (PATCH name / new_parent_id en el Go).
  const parent = `/${segs.slice(0, -1).join('/')}`;
  const to = name
    ? `${parent === '/' ? '' : parent}/${name}`
    : `${toPath.replace(/\/$/, '')}/${base}`;
  if (to === path) return json({ ok: true }); // no-op (mismo nombre/carpeta)

  try {
    await client.move(path, to);
    return json({ ok: true });
  } catch (e) {
    return fail(e, 'move_failed');
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const path = new URL(ctx.request.url).searchParams.get('path');
  if (!path) return json({ error: 'missing_path' }, 400);
  try {
    await client.remove(path);
    return json({ ok: true });
  } catch (e) {
    return fail(e, 'delete_failed');
  }
};
