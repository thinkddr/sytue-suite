/*
 * Middleware de Sytue Cast: la organización en el path (ADR 0056) + el estándar
 * de la suite más DOS excepciones.
 *
 * `sinOrg`: lo público no tiene por qué tener organización —`explore` (la
 * portada), `c` (el canal de una organización, que YA lleva la suya en la ruta),
 * `watch` (un vídeo puede ser público), `embed` y `oembed` (los pide un sitio
 * ajeno) y `media` (los ficheros firmados)—, y las páginas de trabajo `upload`,
 * `studio`, `manage` y `moderation`: sus nombres son también slugs válidos y, sin declararlas,
 * `/cast/upload` se leería como «la organización upload». Con org delante
 * (`/cast/<org>/upload`) se reescriben igual que siempre.
 *
 * LA EXCEPCIÓN DE `frame-ancestors`. Toda la suite va con `frame-ancestors
 * 'none'`. El reproductor incrustado es lo ÚNICO de Cast que se mete en un iframe
 * ajeno, así que SOLO `/cast/embed/` se sirve con `frame-ancestors *` y sin
 * `X-Frame-Options`. Esa página no tiene sesión, ni formularios, ni acciones:
 * reproduce un vídeo público o no listado y enlaza a su página.
 *
 * LA EXCEPCIÓN DE CÁMARA Y MICRÓFONO. Emitir desde el navegador necesita cámara,
 * micrófono y captura de pantalla: la misma política que ya concede Rooms, y solo
 * en `/cast/studio`, que es donde se emite.
 */
import { sequence } from 'astro:middleware';
import { middlewareDeIdioma, middlewareDeOrganizacion } from '@sytue/config/astro-middleware';
import { SYTUE_PERMISSIONS_POLICY_ROOMS } from '@sytue/config/astro-security';
import { bff, toBffContext } from './lib/bff.js';
import { cabecerasDeRuta } from './lib/cabeceras.js';
import { PAGINAS_SIN_ORG } from './lib/rutas.js';

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/cast',
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
    sinOrg: [...PAGINAS_SIN_ORG],
  }),
  middlewareDeIdioma((context) =>
    cabecerasDeRuta(context.url.pathname, SYTUE_PERMISSIONS_POLICY_ROOMS),
  ),
);
