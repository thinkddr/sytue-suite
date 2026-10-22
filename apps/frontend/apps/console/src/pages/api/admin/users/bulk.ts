/*
 * BFF de Sytue Admin — la misma acción sobre varios usuarios a la vez.
 *
 *   POST /api/admin/users/bulk   body { action, user_ids[], role? }
 *       action ∈ suspend | reactivate | revoke_sessions | set_role.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/users/bulk, Go) con el access token
 * httpOnly; exige rol admin/owner y `users.manage` —el mismo techo que la acción
 * individual, porque un lote no puede ser la vía para saltarse lo que de uno en
 * uno no se deja hacer—. Sytue ID responde 200 con el detalle POR USUARIO incluso
 * si fallan todos, así que aquí se devuelve tal cual: aplanarlo a éxito/fracaso
 * obligaría a la pantalla a adivinar a quiénes se les aplicó.
 *
 * Esta ruta faltaba: la barra de selección del directorio lleva desde su primer
 * día pidiendo a una URL que no existía, así que marcar a treinta personas y
 * pulsar «Suspender» devolvía un 404 y un «no se ha podido aplicar» — sin que
 * nada dijera que la función entera estaba desconectada del servicio que sí la
 * tenía implementada.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { bulkUsers, proxyError, proxyStatus } from '../../../../lib/admin.js';

export const prerender = false;

const ACTIONS = ['suspend', 'reactivate', 'revoke_sessions', 'set_role'] as const;
type BulkAction = (typeof ACTIONS)[number];

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }

  const body = (await ctx.request.json().catch(() => null)) as {
    action?: unknown;
    user_ids?: unknown;
    role?: unknown;
  } | null;
  const action = body?.action;
  if (typeof action !== 'string' || !ACTIONS.includes(action as BulkAction)) {
    return json({ error: 'bad_request', message: `action ∈ ${ACTIONS.join(' | ')}` }, 400);
  }
  const ids = Array.isArray(body?.user_ids)
    ? body.user_ids.filter((v): v is string => typeof v === 'string' && v !== '')
    : [];
  if (ids.length === 0) return json({ error: 'bad_request', message: 'user_ids vacío' }, 400);
  const role = typeof body?.role === 'string' ? body.role : '';

  try {
    return json(await bulkUsers(guard.session, action as BulkAction, ids, role));
  } catch (e) {
    // El motivo del servicio se conserva: «Solo un owner puede asignar el rol
    // owner» es lo único accionable de un 403 aquí.
    return json(proxyError(e), proxyStatus(e));
  }
};
