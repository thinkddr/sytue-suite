/*
 * Middleware de Sytue Calendar: la organización en el path (ADR 0056) + el
 * estándar de la suite.
 *
 * `middlewareDeOrganizacion` atiende `calendar.sytue.eu/calendar/<org>/...`: una
 * URL con org se reescribe por dentro a la ruta real (las páginas siguen donde
 * estaban) y deja la org en `locals.org`; una URL sin org CON sesión salta a la
 * canónica; sin sesión pasa, como siempre. La org del path NO autoriza: quién
 * puede ver qué evento lo sigue decidiendo el token y el servicio.
 *
 * Sin `sinOrg` propias: esta app no tiene páginas públicas —la vista, el
 * compositor por deep-link y el enlace canónico /events/{id} son para-sesión, y
 * la exportación .ics es un proxy BFF bajo /api/calendar-export—, así que basta
 * la lista común del middleware compartido (`api`, `auth`, `healthz`, `sw.js`,
 * `_astro`). El enlace de reunión de Rooms que viaja en las invitaciones es otra
 * cosa: host público de Rooms sin org, para invitados que quizá no la tienen.
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
    base: '/calendar',
    sinOrg: ['events'],
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
  }),
  middlewareDeIdioma(undefined, () =>
    cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL),
  ),
);
