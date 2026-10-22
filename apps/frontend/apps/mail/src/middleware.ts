/*
 * Middleware de Sytue Mail: la organización en el path (ADR 0056) + el estándar
 * de la suite.
 *
 * `middlewareDeOrganizacion` atiende `mail.sytue.eu/mail/<org>/...`: una URL con
 * org se reescribe por dentro a la ruta real (las páginas siguen donde estaban)
 * y deja la org en `locals.org`; una URL sin org CON sesión salta a la canónica;
 * sin sesión pasa, como siempre. La org del path NO autoriza: quién puede leer
 * qué correo lo sigue decidiendo el token y el servicio.
 *
 * Sin `sinOrg` propias: esta app no tiene páginas públicas —la bandeja y el
 * enlace canónico /messages/{id} son para-sesión, y los adjuntos son un proxy
 * BFF bajo /api/—, así que basta la lista común del middleware compartido
 * (`api`, `auth`, `healthz`, `sw.js`, `_astro`).
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
    base: '/mail',
    sinOrg: ['messages'],
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
  }),
  // CSP cerrada (FE-03), medida en producción: su origen y el Hub.
  // Las imágenes remotas que la persona desbloquea pasan por `/api/remote-image`,
  // sin abrir comodín de esquema en las imágenes.
  middlewareDeIdioma(undefined, () =>
    cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL),
  ),
);
