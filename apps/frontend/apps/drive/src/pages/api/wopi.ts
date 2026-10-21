/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Drive — abrir un fichero en el editor externo (WOPI).
 *
 *   GET  /api/wopi            → { enabled, extensions } del editor configurado
 *   POST /api/wopi  { id, mode? } → { editorUrl, accessToken, accessTokenTtl }
 *
 * El servicio Go es el HOST WOPI: acuña un token acotado a ESE fichero y compone la URL
 * del editor a partir del `discovery.xml` que este publica. Aquí no se inventa ninguna
 * URL ni se toca el token: el navegador lo necesita para el POST de formulario que abre
 * el editor, y por eso —a diferencia del Bearer de la sesión— sí baja al cliente. Es
 * seguro porque no da acceso a nada más que a ese fichero y caduca.
 *
 * Sin editor configurado (`enabled:false`) la UI no ofrece la acción, en vez de llevar a
 * una pantalla en blanco.
 */
import type { APIRoute } from 'astro';
import { driveFetch } from '../../lib/auth';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const res = await driveFetch(ctx, '/api/v1/wopi/capabilities');
  if (!res) return json({ error: 'unauthenticated' }, 401);
  // Que el servicio no sepa responder no debe romper la vista: sin editor, sin acción.
  if (!res.ok) return json({ enabled: false, extensions: [] });
  const data = (await res.json()) as { enabled?: boolean; extensions?: string[] };
  return json({ enabled: data.enabled === true, extensions: data.extensions ?? [] });
};

export const POST: APIRoute = async (ctx) => {
  let body: { id?: unknown; mode?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const id = typeof body.id === 'string' ? body.id.trim() : '';
  if (!id) return json({ error: 'missing_id' }, 400);
  const mode = body.mode === 'view' ? '?mode=view' : '';

  const res = await driveFetch(ctx, `/api/v1/files/${encodeURIComponent(id)}/wopi-token${mode}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: '{}',
  });
  if (!res) return json({ error: 'unauthenticated' }, 401);
  if (!res.ok) return json({ error: 'wopi_unavailable' }, res.status >= 500 ? 502 : res.status);

  const data = (await res.json()) as {
    editor_url?: string;
    access_token?: string;
    access_token_ttl?: number;
  };
  // Sin `editor_url` no hay a dónde ir: el editor no declara saber abrir este tipo.
  if (!data.editor_url || !data.access_token) return json({ error: 'no_editor' }, 409);
  return json({
    editorUrl: data.editor_url,
    accessToken: data.access_token,
    accessTokenTtl: data.access_token_ttl ?? 0,
  });
};
