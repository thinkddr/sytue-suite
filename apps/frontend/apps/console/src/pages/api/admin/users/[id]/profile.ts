/*
 * BFF de Sytue Admin — departamento, puesto y ubicación de un miembro.
 *
 *   PATCH /api/admin/users/:id/profile   body { department, title, location }
 *
 * Son los tres atributos con los que se escriben las reglas de los grupos
 * dinámicos, y hasta ahora no había forma de fijarlos: la pantalla de grupos
 * dejaba construir «departamento = Ventas» y la vista previa devolvía cero
 * personas para siempre, porque el único que escribía esos campos era el
 * aprovisionamiento SCIM y está apagado.
 *
 * Proxya a Sytue ID (PATCH /api/v1/admin/users/:id/profile, Go), que los guarda y
 * REEVALÚA los grupos dinámicos del usuario en la misma transacción. El token
 * nunca sale al cliente; se propaga el status del servicio (400 si esa persona no
 * es de la organización, 403 sin permiso).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { setUserAttributes, proxyError, proxyStatus } from '../../../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Texto de un atributo: se manda siempre el trío, y '' borra. La normalización
 *  (espacios, longitud) la hace Sytue ID, que es quien decide qué se guarda. */
function attr(v: unknown): string {
  return typeof v === 'string' ? v.trim() : '';
}

export const PATCH: APIRoute = async (ctx) => {
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
  const id = ctx.params.id;
  if (!id) return json({ error: 'bad_request' }, 400);

  const body = (await ctx.request.json().catch(() => null)) as {
    department?: unknown;
    title?: unknown;
    location?: unknown;
  } | null;

  try {
    return json(
      await setUserAttributes(guard.session, id, {
        department: attr(body?.department),
        title: attr(body?.title),
        location: attr(body?.location),
      }),
    );
  } catch (e) {
    return json(proxyError(e), proxyStatus(e));
  }
};
