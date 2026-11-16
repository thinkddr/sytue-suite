/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /api/primeros-pasos — los «Primeros pasos» que le quedan a la persona de la
 * sesión, juntando `/me`, sus passkeys y sus contraseñas de aplicación de Sytue
 * ID. La portada ya los trae en SSR; esto es para cuando el SSR no pudo (Sytue ID
 * lento), y para quien los quiera releer sin recargar. El token no sale de aquí.
 */
import type { APIRoute } from 'astro';
import { getApiSession, idBaseUrl } from '../../lib/auth';
import { cargarPrimerosPasos } from '../../lib/primeros-pasos';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const pasos = await cargarPrimerosPasos(idBaseUrl(), session.accessToken, ctx.request.signal);
  if (!pasos) return json({ error: 'unavailable' }, 502);
  return json({ pasos });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
