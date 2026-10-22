/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * De un correo a la persona de la organización.
 *
 * Compartir con alguien se escribe siempre igual —poniendo su dirección—, pero
 * los permisos se guardan contra el identificador de usuario (`sub`): una
 * dirección cambia, se reasigna y tiene alias, y un permiso atado a ella dejaría
 * de significar lo mismo. La traducción la hace Sytue ID en
 * `POST /api/v1/directory:resolve`, acotada a la propia organización.
 *
 * VIVE EN EL SERVIDOR Y NO EN EL NAVEGADOR, y esa es la decisión: si el cliente
 * pudiera mandar el `subject`, compartir con quien se quisiera sería escribir un
 * identificador en una petición. Aquí llega un correo y sale un identificador
 * que ha validado el servicio dueño del dato.
 */
import type { APIContext } from 'astro';
import { serverEnv, type EnvLike } from '../config.js';
import type { Bff } from './bff.js';
import { toBffContext } from './app-bff.js';

// Vive en `@sytue/auth/server` desde el 2026-09-25: lo usan Drive y Docs, y copiado
// en cada app diverge solo (docs/CONVENTIONS.md §13.2). Cada app le pasa su BFF.

export interface Persona {
  userId: string;
  email: string;
  name?: string;
}

/** El directorio de la organización de quien llama, con la sesión de su BFF. */
export interface Directorio {
  /**
   * Resuelve `email` a una persona de la organización de quien llama, o null.
   *
   * Null tanto si no existe como si está en otra organización: quien pregunta no
   * tiene por qué distinguir las dos cosas, y distinguirlas convertiría esto en una
   * forma de averiguar dónde trabaja alguien.
   */
  resolverPersona: (ctx: APIContext, email: string) => Promise<Persona | null>;
  /**
   * Lo mismo por su identificador (`sub`): alguien de la organización de quien
   * llama, o null. A diferencia de `resolverPersonas`, NO cae a gente de fuera:
   * es la verja de compartir con alguien por su `sub`.
   */
  resolverPersonaPorId: (ctx: APIContext, userId: string) => Promise<Persona | null>;
  /**
   * Camino de vuelta: del identificador guardado al correo, para poder PINTAR con
   * quién está compartido algo. Sin esto la lista enseñaría identificadores opacos.
   * Resuelve en paralelo y tolera fallos sueltos.
   */
  resolverPersonas: (ctx: APIContext, userIds: readonly string[]) => Promise<Map<string, Persona>>;
  /**
   * Las personas de la organización de quien llama (`GET /api/v1/directory`, hasta
   * 500): el selector de menciones. Vacía en cualquier fallo.
   */
  agenda: (ctx: APIContext) => Promise<Persona[]>;
}

export function crearDirectorio(bff: Pick<Bff, 'getSession'>, buildEnv: EnvLike): Directorio {
  const env = serverEnv(buildEnv);
  /** Raíz del servicio de identidad (el mismo emisor OIDC de la sesión). */
  const idBaseUrl = () =>
    String(env.SYTUE_OIDC_INTERNAL_BASE_URL ?? env.SYTUE_OIDC_ISSUER ?? '').replace(/\/$/, '');

  /** POST a Sytue ID con la sesión de quien llama. Null en cualquier fallo, a propósito. */
  async function preguntar(
    ctx: APIContext,
    ruta: string,
    cuerpo: Record<string, string>,
  ): Promise<Record<string, unknown> | null> {
    const session = await bff.getSession(toBffContext(ctx));
    const base = idBaseUrl();
    if (!session || !base) return null;
    try {
      const res = await fetch(`${base}${ruta}`, {
        method: 'POST',
        headers: {
          authorization: `Bearer ${session.tokens.accessToken}`,
          'content-type': 'application/json',
          accept: 'application/json',
        },
        body: JSON.stringify(cuerpo),
      });
      return res.ok ? ((await res.json()) as Record<string, unknown>) : null;
    } catch {
      return null;
    }
  }

  /** De un correo o un `sub` a la persona de la organización de quien llama. */
  async function resolver(
    ctx: APIContext,
    cuerpo: Record<string, string>,
  ): Promise<Persona | null> {
    const body = await preguntar(ctx, '/api/v1/directory:resolve', cuerpo);
    const userId = typeof body?.user_id === 'string' ? body.user_id : '';
    const email = typeof body?.email === 'string' ? body.email : '';
    if (!userId || !email) return null;
    return {
      userId,
      email,
      ...(typeof body?.name === 'string' && body.name ? { name: body.name } : {}),
    };
  }

  /**
   * Del identificador al NOMBRE de alguien de OTRA organización (`people:resolve`,
   * ADR 0040): quien canjeó desde fuera un enlace de un documento (ADR 0052). Solo
   * nombre o @usuario —nunca su correo ni su organización—, y solo para un `sub` que
   * quien pregunta ya tiene delante, en su propia lista de permisos.
   */
  async function deFuera(ctx: APIContext, userId: string): Promise<Persona | null> {
    const body = await preguntar(ctx, '/api/v1/people:resolve', { user_id: userId });
    if (body?.user_id !== userId) return null;
    const name =
      (typeof body.name === 'string' && body.name) ||
      (typeof body.username === 'string' && body.username ? `@${body.username}` : '');
    return name ? { userId, email: '', name } : null;
  }

  return {
    async agenda(ctx) {
      const session = await bff.getSession(toBffContext(ctx));
      const base = idBaseUrl();
      if (!session || !base) return [];
      try {
        const res = await fetch(`${base}/api/v1/directory?limit=500`, {
          headers: {
            authorization: `Bearer ${session.tokens.accessToken}`,
            accept: 'application/json',
          },
        });
        if (!res.ok) return [];
        const { items = [] } = (await res.json()) as {
          items?: { user_id?: unknown; email?: unknown; name?: unknown }[];
        };
        return items.flatMap((i) =>
          typeof i.user_id === 'string' && typeof i.email === 'string' && i.email
            ? [
                {
                  userId: i.user_id,
                  email: i.email,
                  ...(typeof i.name === 'string' && i.name ? { name: i.name } : {}),
                },
              ]
            : [],
        );
      } catch {
        return [];
      }
    },
    resolverPersona: (ctx, email) => resolver(ctx, { email }),
    resolverPersonaPorId: (ctx, userId) => resolver(ctx, { user_id: userId }),
    async resolverPersonas(ctx, userIds) {
      const unicos = [...new Set(userIds.filter(Boolean))];
      // Primero su organización; quien no está en ella puede ser alguien de fuera
      // con un permiso canjeado (ADR 0052), y de él solo se sabe el nombre.
      const encontradas = await Promise.all(
        unicos.map(async (id) => (await resolver(ctx, { user_id: id })) ?? deFuera(ctx, id)),
      );
      const mapa = new Map<string, Persona>();
      encontradas.forEach((p, i) => {
        const id = unicos[i];
        if (p && id) mapa.set(id, p);
      });
      return mapa;
    },
  };
}
