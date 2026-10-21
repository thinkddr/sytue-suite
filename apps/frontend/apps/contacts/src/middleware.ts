/*
 * Middleware de Sytue Contacts: la organización en el path (ADR 0056) + el
 * estándar de la suite.
 *
 * `middlewareDeOrganizacion` atiende `contacts.sytue.eu/contacts/<org>/...`: una
 * URL con org se reescribe por dentro a la ruta real (las páginas siguen donde
 * estaban) y deja la org en `locals.org`; una URL sin org CON sesión salta a la
 * canónica; sin sesión pasa, como siempre. La org del path NO autoriza: quién
 * puede ver qué contacto lo sigue decidiendo el token y el servicio.
 *
 * Sin `sinOrg` propias: esta app no tiene páginas públicas —la vista y el enlace
 * canónico /contacts/{id} son para-sesión, y no hay vCard público (la foto y la
 * exportación son proxies BFF bajo /api/)—, así que basta la lista común del
 * middleware compartido (`api`, `auth`, `healthz`, `sw.js`, `_astro`).
 *
 * El idioma y las cabeceras de la casa los pone `@sytue/config/astro-middleware`,
 * compartido por las doce superficies. Aquí solo queda la composición.
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
    base: '/contacts',
    sinOrg: ['contacts'],
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
  }),
  middlewareDeIdioma(undefined, () =>
    cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL),
  ),
);
