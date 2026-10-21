/*
 * BFF de Sytue Admin — el token de aprovisionamiento SCIM de esta organización.
 *
 *   POST   /api/admin/scim-tokens        body { label? }  → { token } UNA vez
 *   DELETE /api/admin/scim-tokens?id=…                    → revoca
 *
 * Proxya a Sytue ID (/api/v1/admin/scim/tokens, Go) con el access token httpOnly.
 * El secreto viaja UNA vez, del servicio al navegador de quien lo crea, y no se
 * guarda en ningún sitio del que se pueda releer: si se pierde, se crea otro.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../server/bff.js';
import { createScimToken, proxyStatus, revokeScimToken } from '../../../lib/admin.js';

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
    perms: ['sso.manage'],
  });
  if (guard instanceof Response) return json({ error: 'forbidden' }, 403);
  const b = (await ctx.request.json().catch(() => null)) as { label?: unknown } | null;
  const label = typeof b?.label === 'string' ? b.label.trim().slice(0, 80) : '';
  try {
    return json(await createScimToken(guard.session, label), 201);
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['sso.manage'],
  });
  if (guard instanceof Response) return json({ error: 'forbidden' }, 403);
  const id = ctx.url.searchParams.get('id') ?? '';
  if (!id) return json({ error: 'bad_request' }, 400);
  try {
    await revokeScimToken(guard.session, id);
    return json({ revoked: true });
  } catch (e) {
    return json({ error: 'upstream' }, proxyStatus(e));
  }
};
