import { sequence } from 'astro:middleware';
import {
  cabecerasCerradas,
  middlewareDeIdioma,
  middlewareDeOrganizacion,
} from '@sytue/config/astro-middleware';
import { appDomains } from '@sytue/config';
import { ORIGENES_DE_INCRUSTADOS } from '@sytue/pages-render';
import { bff, toBffContext } from './lib/bff.js';
import { hubBaseUrl } from './lib/pagina.js';
import { PAGINAS_SIN_ORG } from './lib/rutas.js';

/** Lo que el lienzo puede encuadrar: los incrustados del renderizador y Sytue Cast. */
const MARCOS: readonly string[] = [
  ...Object.values(ORIGENES_DE_INCRUSTADOS),
  ...(URL.canParse(appDomains.cast) ? [new URL(appDomains.cast).origin] : []),
];

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/web',
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
    sinOrg: [...PAGINAS_SIN_ORG],
  }),
  // CSP CERRADA (PAGES-PLAN §14), y con motivo propio: esta app pinta DENTRO de su
  // origen, con la sesión delante, la salida del renderizador de webs de clientes —el
  // lienzo del editor, la galería, las miniaturas, lo que propone la IA—. Va en iframes
  // `srcdoc` sin scripts, que HEREDAN esta política: con ella un diseño no puede llamar
  // ni cargar una imagen de un origen ajeno. El Hub es el MISMO que las páginas le pasan
  // a la shell, que es a quien hay que dejar pasar.
  //
  // Y los incrustados de un diseño (vídeo, mapa, Cast) solo de los orígenes que el
  // renderizador sabe pintar (XSS-06): un iframe hacia otro sitio no carga.
  middlewareDeIdioma(undefined, () => cabecerasCerradas(hubBaseUrl(), { marcos: MARCOS })),
);
