/*
 * BFF de Sytue Admin — organizaciones (tenants).
 *
 *   GET  /api/admin/tenants?cursor=&limit=   Página keyset de las orgs que el
 *        admin gestiona (owner/admin). Para «Cargar más» del panel.
 *   POST /api/admin/tenants   body { slug, name }   Crea una org (creador⇒owner).
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/tenants, Go) con el access token
 * httpOnly; propaga el status del servicio (403/409/400/502). Rol admin/owner.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { createTenant, loadTenantsPage, proxyStatus } from '../../../lib/admin.js';

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
  const cursor = ctx.url.searchParams.get('cursor') ?? undefined;
  const { data } = await loadTenantsPage(guard.session, cursor ? { cursor } : {});
  return json(data);
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), { roles: ['admin', 'owner'] });
  if (guard instanceof Response) return guardStatus(guard.status);
  const body = (await ctx.request.json().catch(() => null)) as {
    slug?: unknown;
    name?: unknown;
  } | null;
  const slug = typeof body?.slug === 'string' ? body.slug.trim().toLowerCase() : '';
  const name = typeof body?.name === 'string' ? body.name.trim() : '';
  // Espejo mínimo de la validación para el UX; la política real la aplica Sytue ID.
  if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(slug)) {
    return json({ error: 'bad_request', message: 'Slug inválido.' }, 400);
  }
  if (!name) {
    return json({ error: 'bad_request', message: 'El nombre es obligatorio.' }, 400);
  }
  try {
    return json(await createTenant(guard.session, { slug, name }), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
