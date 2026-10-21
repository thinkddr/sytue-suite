/*
 * BFF de Sytue Admin — grupos gestionables (directorio del tenant activo).
 *
 *   GET  /api/admin/groups                      Lista de grupos con nº de miembros.
 *   POST /api/admin/groups  body { name, description? }   Crea un grupo.
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/groups, Go) con el access token
 * httpOnly; exige rol admin/owner y opera bajo RLS del tenant. Propaga el status
 * del servicio (403/409/400/502). El token nunca sale al cliente. Sin backend
 * vivo el GET degrada al seed (live=false).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { coerceRule, createGroup, loadAdminGroups, proxyStatus } from '../../../lib/admin.js';

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
    perms: ['groups.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { live, data } = await loadAdminGroups(guard.session);
  return json({ live, groups: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['groups.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const body = (await ctx.request.json().catch(() => null)) as {
    name?: unknown;
    description?: unknown;
    membership_rule?: unknown;
  } | null;
  const name = typeof body?.name === 'string' ? body.name.trim() : '';
  const description = typeof body?.description === 'string' ? body.description.trim() : '';
  // La regla se reenvía tal cual; la validación real (ops, atributos, límites) la aplica
  // Sytue ID y su 400 se propaga vía proxyStatus.
  const rule = coerceRule(body?.membership_rule);
  // Espejo mínimo de validación para el UX; la política real la aplica Sytue ID.
  if (!name) return json({ error: 'bad_request', message: 'El nombre es obligatorio.' }, 400);
  try {
    return json(
      await createGroup(guard.session, {
        name,
        ...(description ? { description } : {}),
        ...(rule ? { membership_rule: rule } : {}),
      }),
      201,
    );
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
