/*
 * Middleware de Sytue Notepads: la organización en el path (ADR 0056) + el
 * estándar de la suite con UNA excepción.
 *
 * La organización como segmento de path (`notes.sytue.eu/notes/<org>/…`) la
 * compone el middleware compartido `middlewareDeOrganizacion` PRIMERO: reescribe
 * por dentro las URL con org, manda a la canónica las sin org cuando hay sesión
 * y deja la org en `locals.org`. La org del path NO autoriza: quién puede leer
 * qué cuaderno lo sigue decidiendo el token y el servicio.
 *
 * Sin org se dejan las rutas comunes (api, auth, assets…) más las de esta app:
 *
 * - `p`, los cuadernos PÚBLICOS (`/notes/p/<token>`): la credencial es el token
 *   del enlace y quien lo abre puede no tener cuenta — ni org que escopiar.
 * - `notebooks`, que es ruta de página con nombre de slug válido y colisiona con
 *   el namespace de organizaciones: `/notes/notebooks/42` tratado como org se
 *   reescribiría a `/notes/42` —un 404—. No lleva guard porque la página entera
 *   es un redirect: conserva la org del path (`locals.org`) en el destino y la
 *   canónica entra sola.
 *
 * El idioma y las cabeceras de la casa los pone `@sytue/config/astro-middleware`,
 * compartido por las doce superficies. Aquí solo queda la composición.
 *
 * LA EXCEPCIÓN DE `frame-ancestors`. Toda la suite se sirve con
 * `frame-ancestors 'none'`: nadie mete una superficie con sesión en un iframe
 * ajeno. La vista previa de un documento incrustado SÍ va en un iframe, y por eso
 * —y solo para esa ruta— se permite el encuadre desde el MISMO origen. Esa ruta
 * devuelve HTML exportado y nada más: no tiene sesión propia, ni formularios, ni
 * acciones que robar con un clic.
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
    base: '/notes',
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
    sinOrg: ['p', 'notebooks'],
  }),
  middlewareDeIdioma(
    (context) =>
      context.url.pathname.endsWith('/api/doc-preview')
        ? { 'content-security-policy': CSP_VISTA_PREVIA, 'x-frame-options': 'SAMEORIGIN' }
        : {},
    () => cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL),
  ),
);
