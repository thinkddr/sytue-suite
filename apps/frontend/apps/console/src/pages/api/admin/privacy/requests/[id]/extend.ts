/*
 * BFF de Sytue Admin — prorrogar dos meses el plazo de una solicitud RGPD.
 *
 *   POST /api/admin/privacy/requests/:id/extend   body { reason } → { request }
 *        Una vez, antes de que venza y con motivo: es lo que se le dice a la
 *        persona (art. 12.3).
 *
 * Proxya a Sytue ID con el token httpOnly; owner/admin.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../../server/bff.js';
import { proxyError, proxyStatus } from '../../../../../../lib/admin.js';
import { prorrogarSolicitud } from '../../../../../../lib/privacidad-api.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const id = ctx.params.id;
  const b = (await ctx.request.json().catch(() => null)) as { reason?: unknown } | null;
  const motivo = typeof b?.reason === 'string' ? b.reason.trim() : '';
  if (!id || !motivo) return json({ error: 'bad_request', code: 'reason' }, 400);
  try {
    return json(await prorrogarSolicitud(guard.session, id, motivo));
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
