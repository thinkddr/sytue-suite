/*
 * Middleware de la ofimática (Docus, Calcus, Diapos, CertiPDF): la organización en el
 * path (ADR 0056) + el estándar de la suite más la excepción del WOPI.
 *
 * `middlewareDeOrganizacion` atiende `calcus.sytue.eu/calcus/<org>/...`: una URL con
 * org se reescribe por dentro a la ruta real (las páginas siguen donde estaban) y
 * deja la org en `locals.org`; una URL sin org CON sesión salta a la canónica;
 * sin sesión pasa, como siempre. La org del path NO autoriza: quién puede editar
 * qué documento lo sigue decidiendo el token y el servicio.
 *
 * `sinOrg`:
 *   · `s` — la vista pública de un enlace de compartición (`/s/<token>`): quien
 *     la abre no tiene por qué tener sesión ni organización.
 *   · `wopi` — la página del editor (`/wopi/editor`) que Drive enmarca para
 *     previsualizar: la compone el servicio a partir del `discovery.xml` y sale
 *     SIN org; escopiarla rompería la excepción de `frame-ancestors` de abajo,
 *     que casa por prefijo de ruta.
 *
 * El idioma y las cabeceras de la casa los pone `@sytue/config/astro-middleware`,
 * compartido por las doce superficies. Aquí solo queda lo que es de la ofimática,
 * y cada app de `apps/` lo monta con un reexport en su `src/middleware.ts`.
 */
import { sequence } from 'astro:middleware';
import {
  cabecerasCerradas,
  middlewareDeIdioma,
  middlewareDeOrganizacion,
} from '@sytue/config/astro-middleware';
import { appDomains } from '@sytue/config';
import { serverEnv } from '@sytue/auth/server';
import { bff, docsBaseUrl, hubBaseUrl, toBffContext } from './lib/auth.js';

/** El origen de una URL, o nada si está mal formada (la CSP no tumba la app). */
const origen = (url: string, esquema?: string): string[] =>
  URL.canParse(url) ? [new URL(url).origin.replace(/^https?/, (s) => esquema ?? s)] : [];

/**
 * LA CSP CERRADA de la ofimática (FE-03), medida en producción con
 * `test/e2e/csp-origenes.mjs`: además de la shell, el WebSocket de coedición, que va
 * al servicio de Docs (`wss://docs.<dominio>`, la URL la da `/api/collab`), y las
 * imágenes de Drive, el único origen ajeno que `urlPermitida` admite en un documento.
 */
const cerrada = (): Readonly<Record<string, string>> =>
  cabecerasCerradas(hubBaseUrl(), {
    conectar: origen(docsBaseUrl(), 'wss'),
    imagenes: origen(appDomains.drive),
  });

/**
 * Hosts que pueden enmarcar la pantalla WOPI (`/wopi/editor`).
 *
 * Es la ÚNICA excepción al `frame-ancestors 'none'` de la suite, y existe porque
 * previsualizar un .docx sin salir de la carpeta significa, literalmente, que
 * Sytue Drive pinta el editor dentro de su propia página. Se abre a los hosts
 * admitidos y a nadie más: quien puede mandarte un fichero es quien puede
 * enmarcar el editor que lo abre.
 *
 * La lista es la MISMA que aplica el servicio Go (`WOPI_ALLOWED_HOSTS`), y por
 * eso se lee de la misma variable: si aquí quedara más corta, el navegador
 * bloquearía marcos que el backend sí acepta y la previsualización saldría en
 * blanco sin decir por qué. Vacía, se deduce el Drive de la propia suite.
 */
function hostsQuePuedenEnmarcar(host: string): string[] {
  const declarados = String(serverEnv(import.meta.env).WOPI_ALLOWED_HOSTS ?? '')
    .split(',')
    .map((h) => h.trim().toLowerCase())
    .filter(Boolean);
  if (declarados.length > 0) return declarados;
  // El Drive del mismo dominio que el host de la app (`calcus.sytue.eu` → `drive.sytue.eu`).
  const app = /^(docus|calcus|diapos|certipdf)\./.exec(host);
  return app ? [`drive.${host.slice(app[0].length)}`] : [];
}

export const onRequest = sequence(
  middlewareDeOrganizacion({
    // El prefijo de la app que monta este middleware (`src/middleware.ts` de cada
    // una lo reexporta): lo hornea Vite al construirla.
    base: import.meta.env.BASE_URL,
    sesion: async (ctx) => bff.getSession(toBffContext(ctx)),
    sinOrg: ['s', 'wopi', 'edit', 'open-from-drive', 'pdf', 'trash'],
  }),
  middlewareDeIdioma((context) => {
    if (!context.url.pathname.startsWith('/wopi/')) return {};
    const marcos = hostsQuePuedenEnmarcar(context.url.hostname.toLowerCase());
    return {
      // La misma CSP cerrada, con la única diferencia de quién puede enmarcarla.
      'content-security-policy': (cerrada()['content-security-policy'] ?? '').replace(
        "frame-ancestors 'none'",
        marcos.length > 0
          ? `frame-ancestors 'self' ${marcos.map((h) => `https://${h}`).join(' ')}`
          : "frame-ancestors 'none'",
      ),
      // `X-Frame-Options` solo sabe decir DENY o un único origen: dejarlo puesto
      // anularía la lista que acaba de declarar la CSP en los navegadores que
      // hacen caso a los dos. El valor vacío omite la cabecera.
      'x-frame-options': '',
    };
  }, cerrada),
);
