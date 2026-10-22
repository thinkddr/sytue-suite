/*
 * BFF de Sytue Admin — editar/borrar una etiqueta de sensibilidad.
 *
 *   PATCH  /api/admin/sensitivity-labels/:id   body { …campos a tocar }
 *   DELETE /api/admin/sensitivity-labels/:id
 *
 * Proxya a Sytue ID (PATCH/DELETE /api/v1/admin/sensitivity-labels/:id, Go) con el
 * access token httpOnly; exige owner/admin de ESA org (cross-org ⇒ 404). Semántica
 * PATCH: solo los campos presentes cambian.
 *
 * IMPORTANTE: cambiar o borrar una etiqueta toca la TAXONOMÍA, no el contenido ya
 * etiquetado (lleva un snapshot propio). La UI lo dice antes de guardar.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import {
  deleteSensitivityLabel,
  proxyStatus,
  sanitizeSensitivityLabelInput,
  updateSensitivityLabel,
} from '../../../../lib/admin.js';

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

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  const patch = sanitizeSensitivityLabelInput(await ctx.request.json().catch(() => null));
  try {
    return json(await updateSensitivityLabel(guard.session, id, patch));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  try {
    await deleteSensitivityLabel(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
