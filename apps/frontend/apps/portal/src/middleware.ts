/*
 * Middleware del Portal: la organización en el path (ADR 0056), los iconos de la
 * raíz del host, y lo estándar.
 *
 * La organización como segmento de path (`one.sytue.eu/one/<org>/…`) la compone
 * el middleware compartido `middlewareDeOrganizacion` PRIMERO: reescribe por
 * dentro las URL con org, manda a la canónica las sin org cuando hay sesión y
 * deja la org en `locals.org`. La org del path NO autoriza: eso sigue en el
 * token y en el servicio. Además de lo común (api, auth, assets…), se declaran
 * en `sinOrg` las SIETE rutas de página, porque todas son también slugs válidos
 * (`actividad`, `datos`…): si el middleware las tratara como org, un enlace
 * antiguo a `/one/datos` reescribiría a `/one/?__org=datos` y serviría el
 * lanzador en vez de la página. El guard de cada página compone ahí la
 * canónica con la org de la sesión (`canonicaConOrg`), el mismo salto local que
 * el middleware aplica a lo que no colisiona. «bienvenida», la de los recién
 * llegados, la visitan sin org activa en la sesión: para ellos no hay salto ni
 * org que poner.
 *
 * La negociación de idioma y las cabeceras de seguridad son las de la suite
 * (`@sytue/config/astro-middleware`). Esta app NO usa el handler estándar para
 * el segundo eslabón porque tiene que decidir algo ANTES de entregar la ruta a
 * Astro (la redirección de iconos), así que compone el middleware ella misma y
 * llama a `negociarIdioma` para que el orden de preferencia —cookie >
 * `Accept-Language` > inglés— siga siendo el mismo que en las otras once
 * superficies.
 */
import { sequence } from 'astro:middleware';
import { defineMiddleware } from 'astro:middleware';
import { applySecurityHeaders } from '@sytue/config/astro-security';
import {
  cabecerasCerradas,
  middlewareDeOrganizacion,
  negociarIdioma,
} from '@sytue/config/astro-middleware';
import { withBase } from '@sytue/ui';
import { bff, hubBaseUrl, toBffContext } from './lib/auth.js';

/** CSP cerrada (FE-03): medido en producción, el portal solo habla con su origen y el Hub. */
const cerrada = (): Readonly<Record<string, string>> => cabecerasCerradas(hubBaseUrl());

/**
 * Iconos pedidos A LA RAÍZ DEL HOST sin leer el HTML. Safari cae a `/favicon.ico`
 * cuando no le vale el SVG declarado, y a `/apple-touch-icon.png` para Favoritos
 * y «añadir a inicio»; lectores y previsualizaciones de enlaces hacen lo mismo.
 * En el host de organización nada cuelga de `/` salvo el portal (el Ingress le
 * manda `/` y estas cuatro rutas exactas), así que responde él: una redirección
 * permanente a su icono bajo `/one`, que es la marca de la suite.
 */
const ICONOS_DE_RAIZ: Record<string, string> = {
  '/favicon.ico': '/favicon.ico',
  '/favicon.svg': '/favicon.svg',
  '/apple-touch-icon.png': '/apple-touch-icon.png',
  '/apple-touch-icon-precomposed.png': '/apple-touch-icon.png',
};

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/one',
    sesion: async (context) => bff.getSession(toBffContext(context)),
    // Nombres de página = slugs válidos: sin org por construcción, con la
    // canónica compuesta por el guard de cada página (canonicaConOrg).
    sinOrg: [
      'actividad',
      'buscar',
      'datos',
      'descargas',
      'dispositivos',
      'notificaciones',
      'bienvenida',
    ],
  }),
  defineMiddleware(async (context, next) => {
    const icono = ICONOS_DE_RAIZ[context.url.pathname];
    if (icono) return context.redirect(withBase(import.meta.env.BASE_URL, icono), 301);
    negociarIdioma(context);
    const response = await next();
    applySecurityHeaders(response.headers, {}, cerrada());
    return response;
  }),
);
