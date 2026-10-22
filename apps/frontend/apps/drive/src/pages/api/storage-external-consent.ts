/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import {
  HttpClient,
  HttpError,
  bearer,
  isStorageOAuthKind,
  startStorageConsent,
  upstreamStatus,
} from '@sytue/api-clients';
import { bff, driveFetch, integrationsBaseUrl, toBffContext } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  // La misma puerta de permisos (owner/admin) que Drive y Hub. `available` NO se
  // exige: solo dice si hay KEK para sellar secretos, y Google Drive, OneDrive y
  // Dropbox no guardan ninguno aquí (la credencial vive en la pasarela).
  const access = await driveFetch(ctx, '/api/v1/storage/external');
  if (!access?.ok) return json({ error: 'forbidden' }, access?.status === 401 ? 401 : 403);
  const body = (await ctx.request.json().catch(() => null)) as {
    kind?: unknown;
    instance_id?: unknown;
    new_account?: unknown;
  } | null;
  const kind = body?.kind;
  if (!isStorageOAuthKind(kind)) return json({ error: 'invalid_provider' }, 400);
  if (body?.new_account === true && body.instance_id)
    return json({ error: 'invalid_account' }, 400);
  const base = integrationsBaseUrl();
  const client = new HttpClient({
    baseUrl: `${base.replace(/\/$/, '')}/api/v1`,
    auth: bearer(session.tokens.accessToken),
  });
  try {
    return json(
      await startStorageConsent(
        client,
        kind,
        typeof body?.instance_id === 'string' && body.instance_id ? body.instance_id : undefined,
        { newAccount: body?.new_account === true },
      ),
    );
  } catch (error) {
    if (error instanceof HttpError && error.code === 'oauth_provider_unavailable') {
      return json({ error: 'oauth_provider_unavailable' }, 503);
    }
    return json({ error: 'consent_failed' }, upstreamStatus(error));
  }
};
