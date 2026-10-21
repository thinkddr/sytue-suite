import { sequence } from 'astro:middleware';
import {
  cabecerasCerradas,
  middlewareDeIdioma,
  middlewareDeOrganizacion,
} from '@sytue/config/astro-middleware';
import { bff, toBffContext } from './lib/bff.js';
import { hubBaseUrl } from './lib/pagina.js';
import { PAGINAS_SIN_ORG } from './lib/rutas.js';

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/bases',
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
    sinOrg: [...PAGINAS_SIN_ORG],
  }),
  // CSP CERRADA (ADR 0068): el navegador solo habla con su propio BFF… y con lo que
  // necesita la shell, que es lo que faltaba: sin el Hub, la campana y la presencia no
  // funcionaban aquí. El Hub es el MISMO que las páginas le pasan a la shell.
  middlewareDeIdioma(undefined, () => cabecerasCerradas(hubBaseUrl())),
);
