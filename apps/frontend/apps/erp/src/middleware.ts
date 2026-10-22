/*
 * Middleware de SyERP: la organización en el path (ADR 0056) + el estándar de la
 * suite.
 *
 * `middlewareDeOrganizacion` atiende `erp.sytue.eu/erp/<org>/...`: una URL con
 * org se reescribe por dentro a la ruta real (las páginas siguen donde estaban)
 * y deja la org en `locals.org`; una URL sin org CON sesión salta a la canónica;
 * sin sesión pasa, como siempre. La org del path NO autoriza: quién puede tocar
 * qué catálogo lo sigue decidiendo el token y el servicio.
 *
 * Sin `sinOrg` propias: la única página es la raíz (`/erp`), que no colisiona con
 * el namespace de organizaciones porque no tiene nombre, y todo lo demás es
 * plomería bajo /api y /auth, que la lista común del middleware compartido deja
 * pasar siempre (`api`, `auth`, `healthz`, `sw.js`, `_astro`).
 *
 * El idioma y las cabeceras de la casa los pone `@sytue/config/astro-middleware`,
 * compartido por las trece superficies. Aquí solo queda la composición.
 */
import { sequence } from 'astro:middleware';
import {
  cabecerasCerradas,
  middlewareDeIdioma,
  middlewareDeOrganizacion,
} from '@sytue/config/astro-middleware';
import { serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from './lib/bff.js';

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/erp',
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
  }),
  middlewareDeIdioma(undefined, () =>
    cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL),
  ),
);
