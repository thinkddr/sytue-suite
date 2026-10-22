/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El BFF de «Compartir con una persona», igual en todas las apps que lo tienen
 * (Photos, Notepads): traduce el contrato del diálogo (`CompartirConPersonas` de
 * @sytue/shell) al de los servicios, que es el mismo en todos:
 *
 *   GET    /api/<ruta>?<recurso>                          →  GET    <base>
 *   POST   /api/<ruta>?<recurso> {email|subject, permission} →  PUT    <base>/<sub> {permission}
 *   DELETE /api/<ruta>?<recurso>&sub=<sub>                →  DELETE <base>/<sub>
 *
 * La persona se ESCRIBE por su correo y se GUARDA por su `sub`, como en Drive: la
 * traducción la hace Sytue ID aquí, en el servidor (`crearDirectorio`), porque un
 * `subject` elegido por el navegador sería conceder un permiso a quien se quisiera. Si
 * llega un `sub`, también se pregunta: tiene que ser de la organización. Y el servicio
 * lo vuelve a comprobar antes de guardarlo.
 *
 * La lista vuelve con el correo y el nombre de cada persona: el servicio solo guarda
 * identificadores, y una lista de identificadores no le sirve a nadie para decidir.
 */
import type { APIContext, APIRoute } from 'astro';
import type { ServerSession } from './app-bff.js';
import type { Directorio } from './directorio.js';

export interface OpcionesCompartir {
  /** La sesión de la app (`getApiSession` de `crearBffDeApp`). */
  sesion: (ctx: APIContext) => Promise<ServerSession | null>;
  /** La raíz de la API del servicio, con su `/api/v1`. */
  servicio: () => string;
  /** La ruta de personas del recurso a partir de la query (`/albums/<id>/people`), o null. */
  base: (q: URLSearchParams) => string | null;
  /** El directorio de la app (`crearDirectorio(bff, import.meta.env)`). */
  directorio: Pick<Directorio, 'resolverPersona' | 'resolverPersonaPorId' | 'resolverPersonas'>;
}

/** Una persona de la lista, ya con cara. */
export interface PersonaCompartida {
  subject: string;
  permission: 'read' | 'write';
  email?: string;
  name?: string;
}

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export function rutasDeCompartir(o: OpcionesCompartir): {
  GET: APIRoute;
  POST: APIRoute;
  DELETE: APIRoute;
} {
  /** Llama al servicio con el token de la sesión; null si no hay sesión o recurso. */
  const llamar = async (
    ctx: APIContext,
    metodo: string,
    sufijo: string,
    cuerpo?: unknown,
  ): Promise<Response> => {
    const sesion = await o.sesion(ctx);
    if (!sesion) return json({ error: 'unauthenticated' }, 401);
    const base = o.base(new URL(ctx.request.url).searchParams);
    if (!base) return json({ error: 'bad_request' }, 400);
    try {
      return await fetch(`${o.servicio()}${base}${sufijo}`, {
        method: metodo,
        headers: {
          authorization: `Bearer ${sesion.accessToken}`,
          accept: 'application/json',
          ...(cuerpo === undefined ? {} : { 'content-type': 'application/json' }),
        },
        ...(cuerpo === undefined ? {} : { body: JSON.stringify(cuerpo) }),
      });
    } catch {
      return json({ error: 'upstream_unreachable' }, 502);
    }
  };
  /** La respuesta del servicio tal cual, con su estado (un 404 es «no es tuyo»). */
  const reenviar = async (res: Response): Promise<Response> =>
    new Response(res.status === 204 ? null : await res.text(), {
      status: res.status,
      headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
    });

  return {
    async GET(ctx) {
      const res = await llamar(ctx, 'GET', '');
      if (!res.ok) return reenviar(res);
      const { people = [] } = (await res.json()) as { people?: PersonaCompartida[] };
      const caras = await o.directorio.resolverPersonas(
        ctx,
        people.map((p) => p.subject),
      );
      return json(
        {
          people: people.map((p): PersonaCompartida => {
            const c = caras.get(p.subject);
            return {
              subject: p.subject,
              permission: p.permission === 'write' ? 'write' : 'read',
              ...(c?.email ? { email: c.email } : {}),
              ...(c?.name ? { name: c.name } : {}),
            };
          }),
        },
        200,
      );
    },

    async POST(ctx) {
      let b: { email?: unknown; subject?: unknown; permission?: unknown };
      try {
        b = (await ctx.request.json()) as typeof b;
      } catch {
        return json({ error: 'invalid_body' }, 400);
      }
      const email = typeof b.email === 'string' ? b.email.trim() : '';
      const subject = typeof b.subject === 'string' ? b.subject.trim() : '';
      if (!email && !subject) return json({ error: 'missing_person' }, 400);
      // Sin sesión, el directorio no contesta: que lo diga `llamar` con su 401.
      if (!(await o.sesion(ctx))) return json({ error: 'unauthenticated' }, 401);
      const persona = email
        ? await o.directorio.resolverPersona(ctx, email)
        : await o.directorio.resolverPersonaPorId(ctx, subject);
      if (!persona) return json({ error: 'unknown_person' }, 404);
      const permission = b.permission === 'write' ? 'write' : 'read';
      const res = await llamar(ctx, 'PUT', `/${encodeURIComponent(persona.userId)}`, {
        permission,
      });
      if (!res.ok) return reenviar(res);
      const r = (await res.json()) as { created?: boolean };
      return json(
        {
          person: {
            subject: persona.userId,
            permission,
            email: persona.email,
            ...(persona.name ? { name: persona.name } : {}),
          },
          created: r.created === true,
        },
        200,
      );
    },

    async DELETE(ctx) {
      const sub = new URL(ctx.request.url).searchParams.get('sub');
      if (!sub) return json({ error: 'bad_request' }, 400);
      return reenviar(await llamar(ctx, 'DELETE', `/${encodeURIComponent(sub)}`));
    },
  };
}
