/*
 * BFF de Sytue Admin — editar el perfil del propio usuario (self-service).
 *
 *   PATCH /api/me/profile   body { name }
 *       Actualiza el display name del usuario autenticado. Sytue ID valida/sanea
 *       el nombre y actualiza SOLO al usuario del token (email verificado, rol y
 *       tenant NO se tocan aquí). Cualquier sesión válida basta.
 *
 * Proxya a Sytue ID (PATCH /api/v1/me, Go) con el access token httpOnly; propaga
 * el status del servicio (400/401/…). El token nunca sale al cliente. Ruta BFF
 * dedicada: NO comparte código con el BFF de auth. docs/CONVENTIONS.md §8, §9.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { proxyStatus, updateProfile } from '../../../lib/admin.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => null)) as { name?: unknown } | null;
  const name = typeof body?.name === 'string' ? body.name.trim() : '';
  if (!name) return json({ error: 'bad_request', message: 'El nombre no puede estar vacío.' }, 400);

  try {
    return json(await updateProfile(guard.session, name));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
