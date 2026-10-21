/*
 * BFF de Sytue Admin — un proveedor de servicio SAML de la organización.
 *
 *   PATCH  /api/admin/apps/saml-sps/:id   body { enabled }   Corta o restablece la
 *          federación con esa app sin perder su configuración.
 *   DELETE /api/admin/apps/saml-sps/:id   La app deja de poder federar con la org.
 *
 * Proxya a Sytue ID (PATCH/DELETE /api/v1/admin/apps/saml-sps/:id, Go) y propaga su
 * status. Aislamiento por org en el servicio: el SP de otra organización responde 404.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { deleteSamlSp, proxyStatus, setSamlSpEnabled } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const body = (await ctx.request.json().catch(() => null)) as { enabled?: unknown } | null;
  if (typeof body?.enabled !== 'boolean') {
    return json({ error: 'bad_request', message: 'enabled debe ser booleano.' }, 400);
  }
  try {
    return json(await setSamlSpEnabled(guard.session, ctx.params.id ?? '', body.enabled));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);
  const id = ctx.params.id ?? '';
  try {
    await deleteSamlSp(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
