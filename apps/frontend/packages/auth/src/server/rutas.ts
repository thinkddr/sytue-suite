/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS HANDLERS DE /auth/* DE TODAS LAS APPS, ESCRITOS UNA VEZ.
 *
 * Los siete endpoints del BFF vivían copiados en cada superficie: 82 ficheros y
 * 927 líneas repitiendo `(ctx) => bff.X(toBffContext(ctx))` doce veces. Lo caro
 * no era el tamaño, era que YA HABÍAN DIVERGIDO sin que nadie lo decidiera: unas
 * apps aceptaban GET en el logout con `export const GET = POST` y otras con un
 * segundo wrapper, el JSON de `/auth/token` no era el mismo en todas, y ninguna
 * cabecera de comentario coincidía con la de al lado. Un arreglo en el endpoint
 * de sesión se aplicaba donde el autor se acordaba de mirar, que nunca son doce
 * sitios: así es como se cae el login de UNA superficie y tarda semanas en verse.
 *
 * ESTE MÓDULO NO CONSTRUYE EL BFF, LO RECIBE, y no es pereza: cada app lo cablea
 * distinto A PROPÓSITO y unificarlo aquí cambiaría en silencio qué cookie se
 * emite en producción. Tres diferencias reales, todas medidas en su día:
 *   - el flag de cookie segura se lee de tres variables distintas
 *     (`DEV`/`MODE`, `SYTUE_SECURE_COOKIES`, `SYTUE_COOKIES_SECURE`);
 *   - Cloud y Admin dejan que `SYTUE_BASE_PATH` anule el `base` del build,
 *     porque una misma imagen sirve `/admin` y la raíz de otro host;
 *   - Docs, Drive y Portal construyen la URL con `new URL(request.url)` en vez
 *     de `Astro.url`.
 *
 * EL BFF ENTRA COMO GETTER (`() => Bff`) Y NO COMO VALOR porque Cloud y Admin lo
 * construyen de forma perezosa y cacheada: `resolveOidcConfig` lanza si falta
 * una SYTUE_OIDC_*, y con la construcción ansiosa ese fallo tumba el módulo
 * entero al cargarse —la app no arranca— en lugar de devolver un 500 en el
 * endpoint que lo necesita. Pasar `() => bff` desde las otras diez es gratis.
 *
 * El contexto de Astro entra por genérico: ningún paquete de este monorepo
 * depende de `astro`, y no vamos a empezar por el de autenticación.
 */
import { json, type Bff, type BffContext } from './bff.js';

/**
 * Los handlers de `/auth/*` listos para reexportar desde `src/pages/auth/`.
 *
 * Se devuelven como flechas que llaman `bff.X(...)` con el receptor puesto y NO
 * como referencias sueltas a los métodos: `session` y `organizations` usan
 * `this.getSession(...)` por dentro, así que un `const { session } = bff` los
 * rompe con un `this` undefined en cuanto alguien abre la app.
 */
export function rutasDeAuth<C>(bff: () => Bff, toBffContext: (ctx: C) => BffContext) {
  return {
    /** GET /auth/login */
    login: (ctx: C) => bff().login(toBffContext(ctx)),
    /** GET /auth/callback */
    callback: (ctx: C) => bff().callback(toBffContext(ctx)),
    /**
     * POST|GET /auth/logout.
     *
     * GET OBLIGATORIO EN TODAS LAS SUPERFICIES: el menú de cuenta NAVEGA a
     * `/auth/logout` y el barrido de cookies de Sytue ID llega como subrecurso,
     * ambos GET. Las apps que solo exportaban POST respondían 404 y cerrar
     * sesión no hacía nada: ni borraba su cookie ni llegaba al end_session. Lo
     * vigila `logout-routes.test.ts`.
     */
    logout: (ctx: C) => bff().logout(toBffContext(ctx)),
    /** POST /auth/refresh */
    refresh: (ctx: C) => bff().refresh(toBffContext(ctx)),
    /** GET /auth/session */
    session: (ctx: C) => bff().session(toBffContext(ctx)),
    /** GET|PUT /auth/organizations */
    organizations: (ctx: C) => bff().organizations(toBffContext(ctx)),
    /** GET /auth/avatar — la foto propia, que es a donde apunta `user.picture`. */
    avatar: (ctx: C) => bff().avatar(toBffContext(ctx)),
    /**
     * GET /auth/token — el access token vigente para las islas del navegador.
     *
     * Sale por `getSession`, que refresca de forma transparente si está a punto
     * de caducar. El refresh token NO viaja: se queda en la cookie httpOnly del
     * servidor. `no-store` porque el token no debe tocar ninguna caché.
     * docs/CONVENTIONS.md §11.
     */
    token: async (ctx: C) => {
      const session = await bff().getSession(toBffContext(ctx));
      if (!session) return json({ error: 'unauthenticated' }, 401);
      return json({
        accessToken: session.tokens.accessToken,
        expiresAt: session.tokens.expiresAt,
      });
    },
  };
}
