/*
 * BFF de Sytue Admin — supresión física de la cuenta de una solicitud RGPD.
 *
 *   POST /api/admin/privacy/requests/:id/erase   body { confirm } → { request, note }
 *        `confirm` es el correo de la persona, escrito a mano. No se deshace.
 *
 * Proxya a Sytue ID con el token httpOnly; owner/admin. Sytue ID exige haberse
 * identificado hace poco: su 401 `insufficient_user_authentication` se propaga con
 * su código para que la isla lleve a confirmar la identidad (lib/reautenticar.ts)
 * y vuelva aquí. Los 409 llevan el motivo (cuenta que también es de fuera, única
 * administradora, supresión en curso).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../../server/bff.js';
import { proxyError, proxyStatus } from '../../../../../../lib/admin.js';
import { suprimirCuenta } from '../../../../../../lib/privacidad-api.js';

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
  const b = (await ctx.request.json().catch(() => null)) as { confirm?: unknown } | null;
  const confirmacion = typeof b?.confirm === 'string' ? b.confirm.trim() : '';
  if (!id || !confirmacion) return json({ error: 'bad_request', code: 'confirm' }, 400);
  try {
    return json(await suprimirCuenta(guard.session, id, confirmacion));
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
