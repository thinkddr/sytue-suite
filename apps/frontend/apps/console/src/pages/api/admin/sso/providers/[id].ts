/*
 * BFF de Sytue Admin — editar/borrar un proveedor SSO.
 *
 *   PATCH  /api/admin/sso/providers/:id   body { name?, slug?, issuer?, client_id?,
 *          client_secret?, scopes?, attribute_mapping?, allowed_email_domains?,
 *          enabled? }   Edita el proveedor (si cambia el issuer se re-descubre).
 *   DELETE /api/admin/sso/providers/:id   Borra el proveedor.
 *
 * Proxya a Sytue ID (PATCH/DELETE /api/v1/admin/sso/providers/:id, Go) con el access
 * token httpOnly; exige rol admin/owner de ESA org (cross-org ⇒ 404). El secreto
 * solo se re-escribe si viene en el cuerpo, y jamás vuelve al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../../server/bff.js';
import { deleteSsoProvider, proxyStatus, updateSsoProvider } from '../../../../../lib/admin.js';
import type { SsoProviderInput } from '../../../../../lib/types.js';

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

export const PATCH: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['sso.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  if (!body) return json({ error: 'bad_request' }, 400);
  const patch: Partial<SsoProviderInput> = {};
  if (typeof body.name === 'string') patch.name = body.name.trim();
  if (typeof body.slug === 'string') patch.slug = body.slug.trim().toLowerCase();
  if (typeof body.issuer === 'string') patch.issuer = body.issuer.trim().replace(/\/+$/, '');
  if (typeof body.client_id === 'string') patch.client_id = body.client_id.trim();
  // client_secret: solo se envía si es una cadena (incluida '' para vaciarlo).
  if (typeof body.client_secret === 'string') patch.client_secret = body.client_secret;
  if (typeof body.scopes === 'string') patch.scopes = body.scopes;
  if (body.attribute_mapping && typeof body.attribute_mapping === 'object') {
    patch.attribute_mapping = body.attribute_mapping as Record<string, string>;
  }
  if (Array.isArray(body.allowed_email_domains)) {
    patch.allowed_email_domains = (body.allowed_email_domains as unknown[]).map(String);
  }
  if (typeof body.enabled === 'boolean') patch.enabled = body.enabled;
  // Campos SAML (el tipo del provider es inmutable; Sytue ID solo aplica los suyos).
  for (const k of [
    'metadata_xml',
    'idp_entity_id',
    'idp_sso_url',
    'idp_certificate',
    'sp_entity_id',
    'saml_binding',
    'name_id_format',
  ] as const) {
    if (typeof body[k] === 'string') (patch as Record<string, string>)[k] = body[k];
  }
  try {
    return json(await updateSsoProvider(guard.session, id, patch));
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['sso.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const id = ctx.params.id ?? '';
  try {
    await deleteSsoProvider(guard.session, id);
    return json({ id, deleted: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
