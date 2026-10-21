/*
 * BFF de Sytue Admin — recálculo de TODOS los grupos dinámicos de la org.
 *
 *   POST /api/admin/groups/recompute
 *     → { groups, added, removed }  (efecto REAL del barrido completo)
 *
 * Proxya a Sytue ID (POST /api/v1/admin/groups/recompute, Go) con el access token
 * httpOnly; exige rol admin/owner y groups.manage. Es el hermano de
 * `[id]/recompute.ts`: tras una importación o una reorganización, reevaluar grupo
 * por grupo no es una opción. Sytue ID lo resuelve sin N+1 (una carga de usuarios
 * por org y las K reglas en memoria). El token nunca sale al cliente.
 *
 * Ruta ESTÁTICA hermana de `[id].ts`: Astro prioriza el segmento literal, igual
 * que ya ocurre con `preview.ts`.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { proxyStatus, recomputeAllDynamicGroups } from '../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['groups.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  try {
    return json(await recomputeAllDynamicGroups(guard.session));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
