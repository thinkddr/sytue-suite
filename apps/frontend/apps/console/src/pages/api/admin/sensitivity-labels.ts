/*
 * BFF de Sytue Admin — etiquetas de sensibilidad (paridad Purview / Google Drive
 * labels).
 *
 *   GET  /api/admin/sensitivity-labels   Taxonomía de la org activa (de menos a más
 *        sensible).
 *   POST /api/admin/sensitivity-labels   body { name, rank?, color?, description?,
 *        block_external_share? }   Crea una etiqueta.
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/sensitivity-labels, Go) con el access token
 * httpOnly; propaga el status del servicio (400/403/409). Reservado a owner/admin: la
 * taxonomía de clasificación es gobierno de la organización, como los roles o el
 * cumplimiento. El enforcement de la etiqueta (bloquear enlaces públicos) vive en el
 * producto que guarda el contenido (Drive), no aquí.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import {
  createSensitivityLabel,
  loadSensitivityLabels,
  proxyStatus,
  sanitizeSensitivityLabelInput,
} from '../../../lib/admin.js';
import type { SensitivityLabelInput } from '../../../lib/types.js';

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
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { live, data } = await loadSensitivityLabels(guard.session);
  return json({ live, items: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const input = sanitizeSensitivityLabelInput(await ctx.request.json().catch(() => null));
  if (!input.name) {
    return json({ error: 'bad_request', message: 'name es obligatorio.' }, 400);
  }
  try {
    return json(await createSensitivityLabel(guard.session, input as SensitivityLabelInput), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
