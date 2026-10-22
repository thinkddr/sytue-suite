/*
 * BFF de Sytue Admin — dispositivos de la organización (vista admin).
 *
 *   GET /api/admin/devices   Lista los dispositivos registrados en la org activa.
 *
 * Proxya a Sytue ID (GET /api/v1/admin/devices, Go) con el access token httpOnly. Rol
 * admin/owner. Sytue ID acota por la org del token (aislamiento tenant).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadOrgDevices } from '../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

function guardStatus(s: number): Response {
  return json({ error: s === 403 ? 'forbidden' : 'unauthenticated' }, s === 403 ? 403 : 401);
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.read', 'users.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { live, data } = await loadOrgDevices(guard.session);
  return json({ live, devices: data });
};
