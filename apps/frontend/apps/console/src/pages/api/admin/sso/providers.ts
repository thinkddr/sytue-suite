/*
 * BFF de Sytue Admin — proveedores SSO (federación OIDC upstream).
 *
 *   GET  /api/admin/sso/providers   Lista los proveedores de la org activa (sin
 *        secreto).
 *   POST /api/admin/sso/providers   body { name, slug, issuer, client_id,
 *        client_secret?, scopes?, attribute_mapping?, allowed_email_domains?,
 *        enabled? }   Crea un proveedor (Sytue ID resuelve el discovery del issuer).
 *
 * Proxya a Sytue ID (GET/POST /api/v1/admin/sso/providers, Go) con el access token
 * httpOnly; propaga el status del servicio (400/403/409/502). Rol admin/owner. El
 * client_secret viaja solo servidor→servidor; nunca vuelve al cliente.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { createSsoProvider, loadSsoProviders, proxyStatus } from '../../../../lib/admin.js';
import type { SsoProviderInput } from '../../../../lib/types.js';

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

const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['sso.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const { data } = await loadSsoProviders(guard.session);
  return json({ items: data });
};

export const POST: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['sso.manage'],
  });
  if (guard instanceof Response) return guardStatus(guard.status);
  const body = (await ctx.request.json().catch(() => null)) as Record<string, unknown> | null;
  const str = (v: unknown) => (typeof v === 'string' ? v.trim() : '');
  const isSAML = body?.type === 'saml';
  const input: SsoProviderInput = {
    type: isSAML ? 'saml' : 'oidc',
    name: str(body?.name),
    slug: str(body?.slug).toLowerCase(),
  };
  if (body?.attribute_mapping && typeof body.attribute_mapping === 'object') {
    input.attribute_mapping = body.attribute_mapping as Record<string, string>;
  }
  if (Array.isArray(body?.allowed_email_domains)) {
    input.allowed_email_domains = (body.allowed_email_domains as unknown[]).map(String);
  }
  if (typeof body?.enabled === 'boolean') input.enabled = body.enabled;

  if (isSAML) {
    for (const k of [
      'metadata_xml',
      'idp_entity_id',
      'idp_sso_url',
      'idp_certificate',
      'sp_entity_id',
      'saml_binding',
      'name_id_format',
    ] as const) {
      if (typeof body?.[k] === 'string') (input as unknown as Record<string, string>)[k] = body[k];
    }
  } else {
    input.issuer = str(body?.issuer).replace(/\/+$/, '');
    input.client_id = str(body?.client_id);
    if (typeof body?.client_secret === 'string') input.client_secret = body.client_secret;
    if (typeof body?.scopes === 'string') input.scopes = body.scopes;
  }

  // Espejo mínimo de la validación para el UX; la política real la aplica Sytue ID.
  if (!input.name || !SLUG_RE.test(input.slug)) {
    return json({ error: 'bad_request', message: 'name y slug son obligatorios.' }, 400);
  }
  if (!isSAML && (!input.issuer || !input.client_id)) {
    return json(
      { error: 'bad_request', message: 'issuer y client_id son obligatorios para OIDC.' },
      400,
    );
  }
  try {
    return json(await createSsoProvider(guard.session, input), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
