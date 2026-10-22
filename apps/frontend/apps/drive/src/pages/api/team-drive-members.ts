/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — MIEMBROS de una unidad de equipo (solo manager, gateado en
 * el servicio Go: un rol insuficiente recibe 403; un no-miembro, 404).
 *
 *   GET    /api/team-drive-members?id=<unidad>            → { members: TeamMember[] }
 *   POST   /api/team-drive-members?id=<unidad>            { subject, role } → añade/cambia rol
 *   DELETE /api/team-drive-members?id=<unidad>&subject=…  → quita a un miembro
 *
 * El cliente usa POST JSON / DELETE (octet-stream) para no disparar el checkOrigin
 * CSRF de Astro; el BFF traduce a PUT/DELETE contra el servicio. El token NUNCA
 * llega al cliente.
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

function driveId(ctx: Parameters<APIRoute>[0]): string {
  return new URL(ctx.request.url).searchParams.get('id')?.trim() || '';
}

export const GET: APIRoute = async (ctx) => {
  const id = driveId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  const res = await driveFetch(ctx, `/api/v1/team-drives/${encodeURIComponent(id)}/members`);
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'members_failed' }, res.status >= 500 ? 502 : res.status);
  return json(await res.json());
};

export const POST: APIRoute = async (ctx) => {
  const id = driveId(ctx);
  if (!id) return json({ error: 'missing_id' }, 400);
  let body: { subject?: unknown; role?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const subject = typeof body.subject === 'string' ? body.subject.trim() : '';
  const role = typeof body.role === 'string' ? body.role : '';
  if (!subject || !role) return json({ error: 'missing_fields' }, 400);
  const res = await driveFetch(ctx, `/api/v1/team-drives/${encodeURIComponent(id)}/members`, {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ subject, role }),
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'member_put_failed' }, res.status >= 500 ? 502 : res.status);
  return json(await res.json());
};

export const DELETE: APIRoute = async (ctx) => {
  const params = new URL(ctx.request.url).searchParams;
  const id = params.get('id')?.trim() || '';
  const subject = params.get('subject')?.trim() || '';
  if (!id || !subject) return json({ error: 'missing_fields' }, 400);
  const res = await driveFetch(
    ctx,
    `/api/v1/team-drives/${encodeURIComponent(id)}/members/${encodeURIComponent(subject)}`,
    { method: 'DELETE', headers: { 'content-type': 'application/octet-stream' } },
  );
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'member_remove_failed' }, res.status >= 500 ? 502 : res.status);
  return json({ ok: true });
};
