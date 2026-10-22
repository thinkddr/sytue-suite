/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/app-passwords — la contraseña de aplicación del botón de un clic de
 * «Conectar tus dispositivos» (Thunderbird, Outlook, Android). Es el mismo
 * `POST /api/v1/me/app-passwords` de Sytue ID que usa Mi cuenta; aquí solo se le
 * pone el token de la sesión, como en `apple-profile.ts`.
 *
 * La respuesta trae la contraseña EN CLARO y es la única vez que existe: no se
 * cachea, y la pantalla la enseña una vez. El access token no llega al navegador.
 */
import type { APIRoute } from 'astro';
import { getApiSession, idBaseUrl } from '../../lib/auth';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response(null, { status: 401 });
  let label = '';
  try {
    const body = (await ctx.request.json()) as { label?: unknown };
    label = typeof body.label === 'string' ? body.label.trim() : '';
  } catch {
    return new Response(null, { status: 400 });
  }
  let upstream: Response;
  try {
    upstream = await fetch(`${idBaseUrl()}/api/v1/me/app-passwords`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
      },
      body: JSON.stringify({ label }),
      signal: ctx.request.signal,
    });
  } catch {
    return new Response(null, { status: 502 });
  }
  if (!upstream.ok) return new Response(null, { status: upstream.status === 429 ? 429 : 502 });
  const { password } = (await upstream.json()) as { password?: string };
  return Response.json({ password: password ?? '' }, { headers: { 'cache-control': 'no-store' } });
};
