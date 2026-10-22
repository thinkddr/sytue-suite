/*
 * BFF de Sytue Admin — arrancar el consentimiento OAuth de una conexión.
 *
 *   POST /api/admin/apps/instances/:id/oauth-start   { return_to?, purpose? }
 *
 * Devuelve la URL de autorización del proveedor y los permisos que se van a pedir.
 * No redirige: quien navega es el NAVEGADOR de la persona, y devolver la URL deja
 * que el asistente enseñe antes qué se va a autorizar. `return_to` dice a qué
 * sección de la consola volver ("apps" | "migrations"); el gateway la valida.
 * `purpose: "out"` pide además los permisos de escritura, para llevar contenido
 * de Sytue a esa cuenta: la conexión de siempre solo puede leer.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../../server/bff.js';
import { instanceAction, proxyStatus } from '../../../../../../lib/gateway.js';

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
    perms: ['apps.manage'],
  });
  if (guard instanceof Response)
    return json({ error: 'forbidden' }, guard.status === 403 ? 403 : 401);

  const body = (await ctx.request.json().catch(() => ({}))) as {
    return_to?: unknown;
    purpose?: unknown;
  };
  const returnTo = typeof body.return_to === 'string' ? body.return_to : '';
  try {
    const res = await instanceAction(guard.session, ctx.params.id ?? '', 'oauth/start', 'POST', {
      return_to: returnTo,
      ...(body.purpose === 'out' ? { purpose: 'out' } : {}),
    });
    return json(res);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
