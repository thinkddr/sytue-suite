/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las versiones de un PDF de CertiPDF (ver `lib/pdfVersiones.ts`):
 *
 *   GET  /api/pdf-versiones?path=…[&cursor=…]   → { versions, nextCursor? }
 *   GET  /api/pdf-versiones?path=…&version=N    → el PDF de esa versión (inline)
 *   POST /api/pdf-versiones  { path, version }  → la restaura → { ok }
 *
 * El token no toca el navegador: habla con Drive este puente, con la sesión.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer } from '@sytue/api-clients';
import { bff, toBffContext, driveBaseUrl } from '../../lib/auth';
import { listarOAbrir, restaurar } from '../../lib/pdfVersiones';

export const prerender = false;

function sinSesion(): Response {
  return new Response(JSON.stringify({ error: 'unauthenticated' }), {
    status: 401,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function cliente(ctx: Parameters<APIRoute>[0]): Promise<DriveClient | null> {
  const sesion = await bff.getSession(toBffContext(ctx));
  if (!sesion) return null;
  return new DriveClient({ baseUrl: driveBaseUrl(), auth: bearer(sesion.tokens.accessToken) });
}

export const GET: APIRoute = async (ctx) => {
  const c = await cliente(ctx);
  return c ? listarOAbrir(new URL(ctx.request.url), c) : sinSesion();
};

export const POST: APIRoute = async (ctx) => {
  const c = await cliente(ctx);
  return c ? restaurar(ctx.request, c) : sinSesion();
};
