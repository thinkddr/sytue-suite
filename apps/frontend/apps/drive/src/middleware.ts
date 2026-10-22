/*
 * Middleware de Sytue Drive: la organización en el path (ADR 0056) + el estándar
 * de la suite.
 *
 * `middlewareDeOrganizacion` atiende `drive.sytue.eu/drive/<org>/...`: una URL con
 * org se reescribe por dentro a la ruta real (las páginas siguen donde estaban) y
 * deja la org en `locals.org`; una URL sin org CON sesión salta a la canónica;
 * sin sesión pasa, como siempre. La org del path NO autoriza: quién puede abrir
 * qué fichero lo sigue decidiendo el token y el servicio.
 *
 * `sinOrg`: `public` —los enlaces de compartición por token, `/public/<token>`—
 * son PARA ANÓNIMOS: quien los abre no tiene por qué tener sesión ni organización.
 * Hoy los sirve el servicio Go (el ingress manda `/public` al backend, no aquí),
 * así que declararlo es la barrera que falta para el día en que esa ruta cambie de
 * manos: el middleware no debe escopiarla nunca. El editor WOPI no tiene página
 * propia en esta app (solo `pages/api/wopi.ts`, bajo `/api/`, ya fuera de escopia);
 * el extremo `/wopi/files` del host es del servicio Go.
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
import { bff, hubBaseUrl, toBffContext } from './lib/auth.js';

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/drive',
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
    sinOrg: ['public', 'files'],
  }),
  middlewareDeIdioma(undefined, () => cabecerasCerradas(hubBaseUrl())),
);
