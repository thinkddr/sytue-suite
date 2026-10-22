/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — UNIDADES DE EQUIPO (gestión de la unidad).
 *
 *   GET  /api/team-drives                       → { items: TeamDrive[] } (mis unidades + rol)
 *   POST /api/team-drives  { name }             → crea una unidad (creador = manager)
 *   POST /api/team-drives  { id, name }         → renombra (solo manager)
 *   POST /api/team-drives  { id, archived }     → archiva/restaura (solo manager)
 *
 * Proxya al servicio Go (drive.sytue.eu) con el access token de la sesión httpOnly.
 * El cliente usa POST JSON para las mutaciones (no dispara el checkOrigin CSRF de
 * Astro); el BFF traduce a PATCH contra el servicio. El token NUNCA llega al cliente.
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
  const res = await driveFetch(ctx, '/api/v1/team-drives');
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'list_failed' }, res.status >= 500 ? 502 : res.status);
  return json(await res.json());
};

export const POST: APIRoute = async (ctx) => {
  let body: { id?: unknown; name?: unknown; archived?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const id = typeof body.id === 'string' ? body.id.trim() : '';

  // Sin id: crear una unidad nueva.
  if (!id) {
    const name = typeof body.name === 'string' ? body.name.trim() : '';
    if (!name) return json({ error: 'missing_name' }, 400);
    const res = await driveFetch(ctx, '/api/v1/team-drives', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    if (!res) return json({ error: 'unauthenticated' }, 401);
    if (!res.ok) return json({ error: 'create_failed' }, res.status >= 500 ? 502 : res.status);
    return json(await res.json(), 201);
  }

  // Con id: renombrar (name) o archivar (archived) — exactamente uno.
  let patch: { name?: string; archived?: boolean };
  if (typeof body.name === 'string') patch = { name: body.name.trim() };
  else if (typeof body.archived === 'boolean') patch = { archived: body.archived };
  else return json({ error: 'nothing_to_update' }, 400);

  const res = await driveFetch(ctx, `/api/v1/team-drives/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(patch),
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'update_failed' }, res.status >= 500 ? 502 : res.status);
  return json(await res.json());
};
