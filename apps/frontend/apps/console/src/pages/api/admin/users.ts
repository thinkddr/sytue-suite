/*
 * BFF de Sytue Admin — directorio de usuarios (paginación keyset).
 *
 *   GET /api/admin/users[?cursor=<opaco>&limit=N&tenant=<slug>&q=<texto>&filter=<estado>]
 *       Página siguiente del directorio de la organización activa. Devuelve
 *       { live, items, nextCursor } con nextCursor '' al agotar. `filter` es uno
 *       de los estados de lib/filtros-usuarios.ts; otro valor es un 400.
 *
 * Proxya al endpoint admin de Sytue ID (GET /api/v1/admin/users, Go) con el
 * access token de la sesión httpOnly; exige rol admin/owner. El token NUNCA
 * llega al cliente. Sin backend vivo degrada al seed (live=false, sin cursor).
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { loadUsers } from '../../../lib/admin.js';
import { esFiltroUsuarios, filtroDeParametro } from '../../../lib/filtros-usuarios.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.read', 'users.manage'],
  });
  // Es una ruta XHR: JSON con el status, nunca la redirección HTML a /auth/login.
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }

  const params = new URL(ctx.request.url).searchParams;
  const cursor = params.get('cursor')?.trim() || '';
  const limit = Number.parseInt(params.get('limit') ?? '', 10);
  const tenant = params.get('tenant')?.trim() || undefined;
  const q = params.get('q')?.trim() || '';
  const filtroPedido = params.get('filter')?.trim() || '';
  // Un filtro desconocido no se convierte en «todas»: quien pide las suspendidas
  // con una errata y recibe a todo el mundo cree que lo están todas.
  if (filtroPedido && !esFiltroUsuarios(filtroPedido)) {
    return json({ error: 'invalid_filter' }, 400);
  }
  const filter = filtroDeParametro(filtroPedido);

  // El servicio Go valida el cursor (opaco), acota limit (1..200) y recorta `q`;
  // aquí solo se descartan valores sin sentido para no propagar basura.
  const { live, data } = await loadUsers(guard.session, tenant, {
    ...(cursor ? { cursor } : {}),
    ...(Number.isFinite(limit) && limit > 0 ? { limit } : {}),
    ...(q ? { q } : {}),
    ...(filter ? { filter } : {}),
  });
  return json({ live, items: data.items, nextCursor: data.nextCursor });
};
