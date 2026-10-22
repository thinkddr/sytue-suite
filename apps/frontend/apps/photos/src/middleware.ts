/*
 * Middleware de Sytue Photos: la organización en el path (ADR 0056) + el estándar
 * de la suite más UNA excepción.
 *
 * `middlewareDeOrganizacion` atiende `photos.sytue.eu/photos/<org>/...`: una URL
 * con org se reescribe por dentro a la ruta real (las páginas siguen donde
 * estaban) y deja la org en `locals.org`; una URL sin org CON sesión salta a la
 * canónica; sin sesión pasa, como siempre. La org del path NO autoriza: qué
 * fotos puede ver cada quien lo sigue decidiendo el token y el servicio.
 *
 * `sinOrg`: `p` — el álbum público de un enlace (`/p/<token>` y sus imágenes):
 * quien lo abre no tiene por qué tener sesión ni organización.
 *
 * El idioma y las cabeceras de la casa los pone `@sytue/config/astro-middleware`,
 * compartido por las doce superficies. Aquí solo queda lo que es de esta app.
 *
 * LA EXCEPCIÓN DE `frame-ancestors`. Toda la suite se sirve con
 * `frame-ancestors 'none'`: nadie mete una superficie con sesión en un iframe
 * ajeno. La vista previa de un documento incrustado SÍ va en un iframe, y por eso
 * —y solo para esa ruta— se permite el encuadre desde el MISMO origen. Esa ruta
 * devuelve HTML exportado y nada más: no tiene sesión propia, ni formularios, ni
 * acciones que robar con un clic. Vive bajo `/api/`, que está fuera de escopia
 * por la lista común, así que la org en el path no la afecta.
 */
import { sequence } from 'astro:middleware';
import {
  cabecerasCerradas,
  middlewareDeIdioma,
  middlewareDeOrganizacion,
} from '@sytue/config/astro-middleware';
import { serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from './lib/bff.js';

/** CSP de la vista previa incrustada: encuadrable desde aquí, y nada más. */
const CSP_VISTA_PREVIA = [
  "object-src 'none'",
  "base-uri 'self'",
  "frame-ancestors 'self'",
  "form-action 'none'",
  "script-src 'none'",
].join('; ');

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/photos',
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
    sinOrg: [
      'p',
      'abrir',
      'album',
      'albums',
      'duplicates',
      'edit',
      'favorites',
      'nearby',
      'shared',
      'similar',
      'trash',
    ],
  }),
  middlewareDeIdioma(
    (context) =>
      context.url.pathname.endsWith('/api/doc-preview')
        ? { 'content-security-policy': CSP_VISTA_PREVIA, 'x-frame-options': 'SAMEORIGIN' }
        : {},
    () => cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL),
  ),
);
