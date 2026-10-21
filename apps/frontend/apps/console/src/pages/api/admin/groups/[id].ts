/*
 * BFF de Sytue Admin — un grupo gestionable.
 *
 *   GET    /api/admin/groups/:id                 Detalle + miembros del grupo.
 *   PATCH  /api/admin/groups/:id  body { name?, description? }   Edita el grupo.
 *   DELETE /api/admin/groups/:id                 Borra el grupo.
 *
 * Proxya a Sytue ID (GET/PATCH/DELETE /api/v1/admin/groups/:id, Go) con el access
 * token httpOnly; exige rol admin/owner bajo RLS del tenant. Propaga el status del
 * servicio (403/404/409). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  coerceRule,
  deleteGroup,
  loadGroupDetail,
  updateGroup,
  proxyStatus,
} from '../../../../lib/admin.js';
import type { MembershipRule } from '../../../../lib/types.js';

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
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    return json(await loadGroupDetail(guard.session, id));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['groups.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  const body = (await ctx.request.json().catch(() => null)) as {
    name?: unknown;
    description?: unknown;
    membership_rule?: unknown;
  } | null;
  const patch: {
    name?: string;
    description?: string;
    membership_rule?: MembershipRule;
  } = {};
  if (typeof body?.name === 'string') patch.name = body.name.trim();
  if (typeof body?.description === 'string') patch.description = body.description.trim();
  if (body && 'membership_rule' in body) {
    const rule = coerceRule(body.membership_rule);
    if (rule) patch.membership_rule = rule;
  }
  if (patch.name === '')
    return json({ error: 'bad_request', message: 'El nombre no puede quedar vacío.' }, 400);
  try {
    return json(await updateGroup(guard.session, id, patch));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['groups.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    await deleteGroup(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
