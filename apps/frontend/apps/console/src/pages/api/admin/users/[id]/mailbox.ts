/*
 * BFF de Sytue Admin — dirección de correo de un miembro y sus alias.
 *
 *   POST   /api/admin/users/:id/mailbox   body { address }  Cambia la principal.
 *   PUT    /api/admin/users/:id/mailbox   body { address }  Añade un alias.
 *
 * Las dos acciones comparten ruta porque son la misma cosa —dar una dirección a
 * alguien— y solo cambia si pasa a ser la principal o una más. Al cambiar la
 * principal, Sytue ID conserva la anterior como alias: el correo que ya iba a ella
 * sigue entregándose.
 *
 * Proxya a Sytue ID (POST /api/v1/admin/users/:id/mailbox y .../aliases, Go) con el
 * access token httpOnly; propaga el status del servicio (409 si la dirección ya
 * está cogida). El token nunca sale al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { setUserMailbox, addUserAlias, proxyStatus } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Guard + lectura de la dirección: lo común a las dos acciones. */
async function preparar(
  ctx: Parameters<APIRoute>[0],
): Promise<
  Response | { session: Parameters<typeof setUserMailbox>[0]; id: string; address: string }
> {
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
  const id = ctx.params.id ?? '';
  if (!id) return json({ error: 'bad_request' }, 400);
  const body = (await ctx.request.json().catch(() => null)) as { address?: unknown } | null;
  const address = typeof body?.address === 'string' ? body.address.trim() : '';
  // Espejo mínimo para el UX; la normalización real (tildes, espacios, dominio) la
  // aplica Sytue ID, que es quien decide si la dirección vale.
  if (!address) {
    return json({ error: 'bad_request', message: 'Escribe una dirección.' }, 400);
  }
  return { session: guard.session, id, address };
}

export const POST: APIRoute = async (ctx) => {
  const p = await preparar(ctx);
  if (p instanceof Response) return p;
  try {
    return json(await setUserMailbox(p.session, p.id, p.address));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const PUT: APIRoute = async (ctx) => {
  const p = await preparar(ctx);
  if (p instanceof Response) return p;
  try {
    return json(await addUserAlias(p.session, p.id, p.address), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
