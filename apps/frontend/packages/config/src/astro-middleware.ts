/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MIDDLEWARE DE IDIOMA Y SEGURIDAD DE LA SUITE, en un solo sitio.
 *
 * Cada app necesita resolver el idioma UNA vez por petición, en el servidor, y
 * dejarlo donde el layout y las islas puedan leerlo. Se hace aquí y no en cada
 * `.astro` porque el idioma tiene que ser el mismo para el `<html lang>`, para el
 * cromo del shell y para lo que pinte cualquier página: resolverlo dos veces es
 * pedir que un día no coincidan.
 *
 * El orden lo fija `negotiateLocale`: cookie de preferencia explícita > cabecera
 * `Accept-Language` del navegador > inglés (idioma base, docs/CONVENTIONS.md §13.1).
 * La cookie manda porque es una decisión que el usuario ha tomado a propósito, y
 * pesa más que la configuración de su navegador.
 *
 * POR QUÉ ESTÁ AQUÍ Y NO COPIADO DOCE VECES. La cabecera del fichero original
 * decía, literalmente, «copia este fichero tal cual. Que las once negocien igual
 * es la razón de que exista» — y esa instrucción se cumplió: doce `middleware.ts`,
 * de los que solo cinco seguían siendo idénticos byte a byte. Las otras siete
 * habían divergido en comentarios y en el reflow de una frase, que es como
 * empieza a divergir de verdad. Si negociar igual es el objetivo, el código tiene
 * que ser el mismo objeto, no doce que se parecen.
 *
 * `defineMiddleware` de `astro:middleware` no se usa a propósito: es la identidad
 * con tipos, y el módulo virtual solo existe dentro del build de una app. Aquí se
 * tipa con `MiddlewareHandler`, que sí sale del paquete `astro` normal, y así este
 * fichero se puede typechequear suelto.
 */
import type { APIContext, MiddlewareHandler } from 'astro';
import { negotiateLocale } from '@sytue/i18n';
import { applySecurityHeaders, SYTUE_SECURITY_HEADERS } from './astro-security.js';
import { origenesDeLaShell } from './tokens.js';

/**
 * Deja el idioma negociado en `Astro.locals.locale`.
 *
 * Suelta y no solo dentro del middleware completo porque Sytue Admin y el Portal
 * tienen que decidir cosas ANTES de entregar la ruta (un 404 por host, una
 * redirección de icono) y no encajan en el handler estándar. Que compartan al
 * menos la negociación es justamente lo que evita que una superficie acabe
 * mirando la cabecera antes que la cookie.
 */
export function negociarIdioma(context: APIContext): void {
  context.locals.locale = negotiateLocale(
    context.request.headers.get('accept-language'),
    context.request.headers.get('cookie'),
  );
}

/**
 * El middleware estándar: negocia el idioma, entrega la ruta y pone las cabeceras
 * de seguridad de la suite sobre la respuesta final.
 *
 * `cabeceras` decide extras POR PETICIÓN, que es lo que necesitan las tres apps
 * con una excepción legítima al `frame-ancestors 'none'` de la casa (la vista
 * previa incrustada de Notepads y de Sytue Photos, el editor WOPI de Docs) y Rooms
 * con su Permissions-Policy de cámara y micrófono. Se resuelven DESPUÉS de
 * `next()`, con la ruta ya conocida.
 */
export function middlewareDeIdioma(
  cabeceras?: (context: APIContext) => Readonly<Record<string, string>>,
  /** La CSP de la app para TODAS sus rutas (`() => cabecerasCerradas(…)`). Va
   *  aparte de `cabeceras` porque no es una excepción de una ruta: se SUMA a la CSP
   *  que una ruta ya haya puesto (el `sandbox` de un adjunto de Drive, Mail o Rooms)
   *  en vez de pisarla, y una excepción por ruta de `cabeceras` sí la sustituye. */
  base?: () => Readonly<Record<string, string>>,
): MiddlewareHandler {
  return async (context, next) => {
    negociarIdioma(context);
    const response = await next();
    applySecurityHeaders(response.headers, cabeceras?.(context) ?? {}, base?.() ?? {});
    return response;
  };
}

/**
 * LA CSP CERRADA, para las apps cuyo navegador habla con su propio BFF y con una
 * lista MEDIDA de orígenes (FE-03; la lista de cada app sale de
 * `test/e2e/csp-origenes.mjs` contra producción): a las cuatro directivas de la suite les añade las tres que
 * `SYTUE_SECURITY_HEADERS` deja para «cuando cada app tenga su inventario de
 * orígenes». Se usa como `middlewareDeIdioma(undefined, () => cabecerasCerradas(hubBaseUrl()))`.
 *
 * El inventario NO es cero aunque la app no llame a nadie: la SHELL que la envuelve
 * habla con el Hub, con el gateway de integraciones y con Sytue ID. Las dos apps
 * nacieron con la conexión limitada a `'self'` —la segunda, copiando el fichero de la
 * primera— y el navegador rechazaba en cada carga los avisos, su flujo y la presencia,
 * sin un error a la vista (medido en producción el 2026-10-05). Por eso está aquí y no
 * en un `lib/cabeceras.ts` por app: lo que la shell necesita lo dice `origenesDeLaShell`
 * una vez, y `plomeria-compartida.test.ts` para a quien vuelva a escribirla a mano.
 *
 * `hubBaseUrl` es el MISMO valor que la página le pasa a `<SytueShell hubBaseUrl>`.
 * Sigue cerrada: orígenes exactos, ni comodines ni esquemas enteros.
 */
export function cabecerasCerradas(
  hubBaseUrl: string | undefined,
  propios: {
    /** Orígenes que la app encuadra (`frame-src`), si encuadra alguno: los incrustados
     *  de Sytue Web. Sin ellos no se añade la directiva y todo queda como estaba. */
    marcos?: readonly string[];
    /** Orígenes AJENOS a los que habla el navegador de esta app además de la shell
     *  (`connect-src`): el servicio de coedición, el SFU… Medidos, no supuestos. */
    conectar?: readonly string[];
    /** Orígenes de imágenes además de la shell (`img-src`). */
    imagenes?: readonly string[];
  } = {},
): Readonly<Record<string, string>> {
  const shell = origenesDeLaShell(hubBaseUrl);
  const { marcos, conectar = [], imagenes = [] } = propios;
  return {
    'content-security-policy': [
      SYTUE_SECURITY_HEADERS['content-security-policy'],
      "script-src 'self' 'unsafe-inline'",
      ["connect-src 'self'", ...shell.conectar, ...conectar].join(' '),
      ["img-src 'self' data: blob:", ...shell.imagenes, ...imagenes].join(' '),
      ...(marcos ? [["frame-src 'self'", ...marcos].join(' ')] : []),
    ].join('; '),
  };
}

/** El middleware sin excepciones, que es el de la mayoría de las superficies. */
export const onRequest: MiddlewareHandler = middlewareDeIdioma();

/*
 * EL MIDDLEWARE DE ORGANIZACIÓN EN EL PATH (ADR 0056), también en un solo sitio.
 *
 * Cada producto se sirve en su host bajo su prefijo y la organización activa
 * cuelga DENTRO del path: `rooms.sytue.eu/rooms/<org>/channels/x`. Esta fábrica
 * compone con el de idioma con `sequence` de `astro:middleware`
 * (`export const onRequest = sequence(org, idioma)`; un array NO vale: Astro
 * responde 500 a todo), y hace tres cosas:
 *
 *   1. URL CON org: reescribe por dentro a la ruta real de la app (que no se
 *      mueve: las páginas siguen donde estaban) y deja la org en
 *      `locals.org` para quien la quiera. El marcador `__org` en la query hace
 *      al middleware IDEMPOTENTE: si el rewrite reejecuta la cadena, la segunda
 *      pasada ve la marca y sigue, sin bucle y sin importar la semántica
 *      concreta del rewrite del runtime.
 *   2. URL SIN org con sesión: redirige a la forma CON la org de la sesión. Es
 *      el único salto, es local (mismo host, sin IdP) y lo pagan solo las
 *      rutas a las que se llega por un enlace viejo o SSR — el navegador deja
 *      de ver la forma sin org.
 *   3. URL SIN org sin sesión: pasa (la guarda de la app manda al login, y al
 *      volver la redirección del punto 2 entra sola). Y las org no escopiables
 *      (`api`, `auth`, assets, enlaces públicos...) pasan siempre.
 *
 * LA ORG DEL PATH NO AUTORIZA: quién puede hacer qué lo decide el token y el
 * servicio. Si la org del path difiere de la de la sesión NO se redirige — un
 * externo con enlace de otra organización tiene que aterrizar donde está.
 *
 * El interruptor `SYTUE_ORG_PATH=off` apaga el comportamiento entero (pasa y
 * no toca nada): es la válvula de escape operativa si una app aún no lleva
 * bien el segmento, sin desplegar código nuevo.
 */
import { MARCA_ORG, partirRuta, RE_SLUG_ORG, SIN_ORG_SIEMPRE } from './organizacion.js';

/** Sesión leída por quien compone la fábrica: solo hace falta el slug activo. */
// `SytueUser.tenant` declara explícitamente `undefined`: con
// `exactOptionalPropertyTypes` una propiedad opcional sin esta unión no admite
// un objeto de sesión que la traiga presente pero sin valor.
export type SesionOrganizacion = { user: { tenant?: string | undefined } } | null | undefined;

export interface OpcionesOrganizacion {
  /** Prefijo de la app en el host (`/rooms`), el `base` de Astro. */
  base: string;
  /** Lee la sesión del BFF de la app; solo se usa el slug activo (`user.tenant`). */
  sesion: (context: APIContext) => Promise<SesionOrganizacion>;
  /**
   * Prefijos de ESTA app que no se escopían, además de los comunes: las páginas
   * públicas y de alta sin org (`s` de los enlaces de Docs, `public` de Drive,
   * `p` de Fotos, los app-links de Rooms...). Una ruta de página que se llame
   * igual que aquí pierde su org — elegir nombres que no choquen.
   */
  sinOrg?: string[];
}

export function middlewareDeOrganizacion(opciones: OpcionesOrganizacion): MiddlewareHandler {
  const base = opciones.base.replace(/\/+$/, '');
  const sinOrg = new Set([...SIN_ORG_SIEMPRE, ...(opciones.sinOrg ?? [])]);
  return async (context, next) => {
    // Válvula operativa: sin apagar el navegador ni el despliegue.
    if (process.env['SYTUE_ORG_PATH'] === 'off') return next();
    const url = context.url;
    // La app también se sirve en la raíz del host (el mismo build atiende
    // `console.<dominio>` y la consola de plataforma): ahí no hay prefijo que
    // escopiar y el comportamiento de siempre se conserva.
    if (!url.pathname.startsWith(`${base}/`) && url.pathname !== base) return next();

    const ruta = url.pathname.slice(base.length).replace(/^\/+/, '');
    const primerSegmento = ruta.split('/')[0] ?? '';
    if (sinOrg.has(primerSegmento)) return next();

    // La marca acredita que ya pasamos por aquí: el rewrite interno llega a la
    // ruta SIN org pero trae `?__org=<org>`, así que —se reejecute el middleware
    // o no, que depende del runtime— resolvemos y servimos, sin bucle.
    const marca = url.searchParams.get(MARCA_ORG);
    if (marca && RE_SLUG_ORG.test(marca)) {
      (context.locals as unknown as Record<string, unknown>).org = marca;
      return next();
    }

    const { org, resto } = partirRuta(ruta);

    // URL CON org: reescribir por dentro a la ruta real —que sigue donde estaba,
    // con su prefijo— llevando la marca y soltando la org en locals. La org va
    // en locals AQUÍ y no solo en la rama de la marca a propósito: si el runtime
    // NO reejecuta la cadena tras el rewrite, esa rama no llega a correr y las
    // páginas verían `locals.org` vacío según el runtime que las sirva.
    if (org) {
      (context.locals as unknown as Record<string, unknown>).org = org;
      const destino = new URL(`${base}/${resto}`, url.origin);
      destino.search = url.search;
      destino.searchParams.set(MARCA_ORG, org);
      return context.rewrite(destino);
    }

    // URL SIN org: con sesión, a la forma canónica (mismo host y query); sin
    // sesión, como siempre — la guarda de la app manda al login y, al volver,
    // esta redirección entra sola.
    const sesion = await opciones.sesion(context).catch(() => null);
    const activa = sesion?.user?.tenant;
    if (!activa) return next();
    const destino = new URL(url.origin);
    destino.pathname = ruta ? `${base}/${activa}/${ruta}` : `${base}/${activa}/`;
    destino.search = url.search;
    return new Response(null, { status: 302, headers: { location: destino.toString() } });
  };
}
