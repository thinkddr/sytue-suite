/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El BFF de «Pedir acceso de edición», igual en todas las apps: traduce el contrato de
 * la franja (`FranjaPeticion` de @sytue/shell) al de platform/pkg/peticiones.
 *
 *   POST   /api/<ruta>?<recurso>             →  POST   <base>
 *   GET    /api/<ruta>?<recurso>             →  GET    <base>
 *   POST   /api/<ruta>?<recurso>&grant=<sub> →  POST   <base>/<sub>/grant
 *   DELETE /api/<ruta>?<recurso>&sub=<sub>   →  DELETE <base>/<sub>
 *
 * `base` la da cada app a partir de la query (`/calendars/<id>/edit-requests`). Quién
 * puede qué lo decide el servicio: aquí solo se traduce la ruta y se pone el token.
 */
import type { APIContext, APIRoute } from 'astro';
import type { ServerSession } from './app-bff.js';

export interface OpcionesPeticiones {
  /** La sesión de la app (`getApiSession` de `crearBffDeApp`). */
  sesion: (ctx: APIContext) => Promise<ServerSession | null>;
  /** La raíz de la API del servicio, con su `/api/v1`. */
  servicio: () => string;
  /** La ruta de peticiones del recurso a partir de la query, o null si falta. */
  base: (q: URLSearchParams) => string | null;
}

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export function rutasDePeticiones(o: OpcionesPeticiones): {
  GET: APIRoute;
  POST: APIRoute;
  DELETE: APIRoute;
} {
  const pasar = async (
    ctx: APIContext,
    metodo: string,
    sufijo: (q: URLSearchParams) => string | null,
  ) => {
    const sesion = await o.sesion(ctx);
    if (!sesion) return json({ error: 'unauthenticated' }, 401);
    const q = new URL(ctx.request.url).searchParams;
    const base = o.base(q);
    const resto = sufijo(q);
    if (!base || resto === null) return json({ error: 'bad_request' }, 400);
    let res: Response;
    try {
      res = await fetch(`${o.servicio()}${base}${resto}`, {
        method: metodo,
        headers: { authorization: `Bearer ${sesion.accessToken}`, accept: 'application/json' },
      });
    } catch {
      return json({ error: 'upstream_unreachable' }, 502);
    }
    return new Response(res.status === 204 ? null : await res.text(), {
      status: res.status,
      headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
    });
  };
  const sub =
    (k: string, cola = '') =>
    (q: URLSearchParams) => {
      const v = q.get(k);
      return v ? `/${encodeURIComponent(v)}${cola}` : null;
    };
  return {
    GET: (ctx) => pasar(ctx, 'GET', () => ''),
    POST: (ctx) => pasar(ctx, 'POST', (q) => (q.get('grant') ? sub('grant', '/grant')(q) : '')),
    DELETE: (ctx) => pasar(ctx, 'DELETE', sub('sub')),
  };
}
