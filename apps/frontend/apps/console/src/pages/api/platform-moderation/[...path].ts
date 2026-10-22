/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de la COLA DE DENUNCIAS DEL OPERADOR — proxy a Sytue Cast y a Sytue Rooms.
 *
 *   GET  /api/platform-moderation/cast/reports?status=open|resolved&cursor=
 *   POST /api/platform-moderation/cast/reports/<uuid>            {"action"}
 *   GET  /api/platform-moderation/rooms/reports?all=1
 *   POST /api/platform-moderation/rooms/reports/<uuid>/resolve   {"action"}
 *
 * La autorización NO está aquí: cada servicio pregunta a Sytue ID en cada
 * petición si quien llama opera la plataforma, con el token de la sesión, y Sytue
 * ID deja la traza (platform/pkg/operador). Además los servicios solo aceptan un
 * token de ESTA consola (`aud` sytue-platform). Este proxy solo existe porque el
 * navegador no ve el token, y lo único que impone es la superficie, la sesión y
 * la allowlist de rutas.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, isPlatformSurface, toBffContext } from '../../../server/bff.js';
import { castBaseUrl, roomsBaseUrl } from '../../../lib/auth.js';
import { CONSULTA_DE_MODERACION, destinoDeModeracion } from '../../../lib/moderacion.js';

export const prerender = false;

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

async function proxy(ctx: Parameters<APIRoute>[0], metodo: string): Promise<Response> {
  if (!isPlatformSurface()) return json({ error: 'not_found' }, 404);
  const guard = await requireSession(getBff(), toBffContext(ctx));
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const destino = destinoDeModeracion(metodo, String(ctx.params.path ?? ''));
  if (!destino) return json({ error: 'not_found' }, 404);

  const entrada = new URL(ctx.request.url).searchParams;
  const salida = new URLSearchParams();
  for (const k of CONSULTA_DE_MODERACION) {
    const v = entrada.get(k);
    if (v !== null) salida.set(k, v);
  }
  const base = destino.servicio === 'cast' ? castBaseUrl() : roomsBaseUrl();
  const qs = salida.toString();
  const cuerpo =
    metodo === 'POST' ? JSON.stringify(await ctx.request.json().catch(() => ({}))) : undefined;
  let res: Response;
  try {
    res = await fetch(
      `${base.replace(/\/$/, '')}/api/v1/platform/moderation/${destino.resto}${qs ? `?${qs}` : ''}`,
      {
        method: metodo,
        headers: {
          authorization: `Bearer ${guard.session.tokens.accessToken}`,
          ...(cuerpo ? { 'content-type': 'application/json' } : {}),
        },
        ...(cuerpo ? { body: cuerpo } : {}),
      },
    );
  } catch {
    return json({ error: 'unavailable' }, 503);
  }
  if (res.status === 204) return new Response(null, { status: 204 });
  const texto = await res.text();
  return new Response(texto || '{}', {
    status: res.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = (ctx) => proxy(ctx, 'GET');
export const POST: APIRoute = (ctx) => proxy(ctx, 'POST');
