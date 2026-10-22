/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA ORGANIZACIÓN COMO SEGMENTO DE PATH (ADR 0056).
 *
 * Cada producto se sirve en su host (`rooms.sytue.eu/rooms/...`) y la
 * organización activa vive DENTRO del path, tras el prefijo de la app:
 * `rooms.sytue.eu/rooms/<org>/channels/x`. El host de organización
 * `<slug>.sytue.eu` queda en transición y su login salta al host de producto.
 *
 * POR QUÉ EL PRIMER SEGMENTO ES SIEMPRE LA ORG. Distinguir «¿esta palabra es una
 * organización o una ruta?» mirando una lista de rutas es pedir que una página
 * nueva rompa el parsing en silencio. En su lugar, por construcción: las URL SIN
 * org son TRANSICIONALES (el middleware redirige a la org activa de la sesión),
 * así que en una URL canónica el primer segmento tras el prefijo SIEMPRE es la
 * organización, salvo los prefijos que cada app declara no escopiables (auth,
 * api, assets, enlaces públicos...). La verja la pone el middleware compartido
 * (`middlewareDeOrganizacion`), no cada app.
 *
 * LA ORG DEL PATH NO AUTORIZA. Viaja en la URL para que un enlace de reunión, un
 * recurso compartido o un aviso digan A QUÉ ORGANIZACIÓN apuntan; quién puede
 * hacer qué lo sigue decidiendo el token (claim `tenant`) y el servicio. Si la
 * org del path y la de la sesión difieren, NO se redirige a ciegas: un externo
 * con enlace de otra organización tiene que aterrizar donde está (el canje y las
 * comparticiones lo resuelven, ADR 0040/0052).
 */
import { stripTrailingSlashes } from './path.js';

/** El formato del slug lo fija platform/pkg/tenancy: `[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?`. */
export const RE_SLUG_ORG = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;

/**
 * Devuelve la organización que encabeza `ruta`, o null si no la lleva.
 *
 * `ruta` es lo que cuelga del prefijo de la app SIN la barra inicial
 * (`acme/channels/x`, `acme`, `channels/x`). Vació o sin slug válido devuelve
 * null: el que llama no tiene que adivinar.
 */
export function orgDeRuta(ruta: string): string | null {
  const primero = ruta.split('/')[0] ?? '';
  return RE_SLUG_ORG.test(primero) ? primero : null;
}

/**
 * Separa la org del resto de la ruta.
 *
 * Devuelve `{ org, resto }` con `resto` SIN la barra inicial (`'edit?id=1'`).
 * Sin org válida, `org` es null y `resto` es la ruta entera pelada. Es la
 * operación inversa de componer, y la usa el middleware para reescribir.
 */
export function partirRuta(ruta: string): { org: string | null; resto: string } {
  const primero = ruta.split('/')[0] ?? '';
  if (!RE_SLUG_ORG.test(primero)) return { org: null, resto: ruta.replace(/^\/+/, '') };
  const resto = ruta.slice(primero.length).replace(/^\/+/, '');
  return { org: primero, resto };
}

/**
 * Compone la ruta CON la organización dentro.
 *
 * `base` es el prefijo de la app (`/rooms`); con org nula o vacía deja la ruta
 * como estaba, que es lo que hacen los llamadores que aún no la conocen.
 */
export function rutaConOrg(base: string, org: string | null | undefined, ruta: string): string {
  const b = stripTrailingSlashes(base);
  const limpia = ruta.startsWith('/') ? ruta : `/${ruta}`;
  if (!org) return `${b}${limpia}`;
  return `${b}/${org}${limpia}`;
}

/** Parámetro interno del rewrite interno del middleware (nunca en un enlace). */
export const MARCA_ORG = '__org';

/**
 * Prefijos que NINGUNA app escopia: plomería del BFF y del navegador. Los lee el
 * middleware (`middlewareDeOrganizacion`) y los helpers de ruta de las apps, que
 * no deben añadir la org a un `/api/...` o un `/auth/...` — la org ahí no cambia
 * nada y el middleware los deja pasar siempre. Vive aquí y no en el módulo del
 * middleware para que también la importe código de navegador: este módulo no
 * depende de Astro ni de nada del servidor.
 */
export const SIN_ORG_SIEMPRE: readonly string[] = [
  'api',
  'auth',
  'healthz',
  'sw.js',
  '_astro',
  // Los ficheros estáticos de marca de `public/` que el navegador pide POR SU
  // CUENTA (favicon, manifiesto PWA, la página offline que precachea el SW):
  // no llevan org —son de todos— y con sesión cada uno costaría un 302 de más
  // en cada carga.
  'favicon.ico',
  'favicon.svg',
  'favicon-64.png',
  'apple-touch-icon.png',
  'manifest.webmanifest',
  'icon-maskable.svg',
  'offline.html',
];

/**
 * La URL canónica de una página servida SIN org, o null si no hay que tocar nada.
 *
 * Para las rutas de página cuyo nombre es TAMBIÉN un slug válido (`channels` de
 * Rooms, `datos` del Portal, `edit` de Docs...): colisionan con el namespace de
 * organizaciones, así que la app las declara en `sinOrg` y el middleware las
 * deja pasar tal cual, con o sin org — no puede distinguir `channels/x` de
 * `<org>/x`. El guard de la página cierra ese hueco: con sesión, la canónica
 * lleva la org activa (un salto local, el mismo que el middleware aplica a las
 * rutas que no colisionan) y las islas solo hidratan en forma canónica. Sin
 * sesión no hay org que poner: la guarda manda al login y, al volver, este
 * salto entra solo.
 *
 * La marca `__org` VÁLIDA en la query acredita que la petición ya llegó por el
 * rewrite con org (la canónica, servida por dentro): en ese caso no hay nada
 * que hacer. Una marca vacía o artesanal (`?__org=`, `?__org=Acme`) no acredita
 * nada y NO bloquea el salto: aceptarla como prueba era dejar la URL fuera de
 * la canónica para siempre. NO se mira `locals.org` a propósito: en rutas sinOrg
 * el middleware puede saltarse la rama de la marca si el runtime reejecuta la
 * cadena, y la marca en la query se ve siempre. Y si la URL no cuelga del
 * prefijo (la app servida en la raíz del host) tampoco: ahí no hay org que
 * escopiar.
 */
export function canonicaConOrg(
  url: URL,
  base: string,
  tenant: string | null | undefined,
): string | null {
  if (!tenant) return null;
  const marca = url.searchParams.get(MARCA_ORG);
  if (marca && RE_SLUG_ORG.test(marca)) return null;
  const b = stripTrailingSlashes(base);
  if (!url.pathname.startsWith(`${b}/`)) return null;
  const resto = url.pathname.slice(b.length) || '/';
  return rutaConOrg(base, tenant, resto + url.search);
}

/**
 * La organización de la URL ACTUAL en el navegador, o null.
 *
 * En SSR no hay URL de navegador y devuelve null: los enlaces que se pintan en
 * el servidor salen sin org y es el middleware de la app el que los manda a la
 * canónica con un 302 local. En el navegador, en cambio, los helpers de ruta
 * (`r()` de cada app) la leen para que la navegación hidratada conserve el
 * segmento y la barra siga enseñando la org en la que se está.
 *
 * LA MARCA MANDA sobre el path. La vuelta del login aterriza en la forma que el
 * rewrite sirve POR DENTRO (`/rooms/channels/x?__org=acme`): el path no lleva
 * org y mirar su primer segmento confundiría una RUTA (`channels`) con ella. La
 * marca es lo que el middleware está sirviendo de verdad, así que si está, esa
 * es la org. En una URL canónica la marca no aparece (el rewrite es interno, la
 * barra del navegador nunca la ve) y el path manda.
 */
export function orgActual(base: string): string | null {
  if (typeof window === 'undefined') return null;
  const marca = new URLSearchParams(window.location.search).get(MARCA_ORG);
  if (marca && RE_SLUG_ORG.test(marca)) return marca;
  const ruta = window.location.pathname;
  const b = stripTrailingSlashes(base);
  if (!ruta.startsWith(`${b}/`)) return null;
  return orgDeRuta(ruta.slice(b.length).replace(/^\/+/, ''));
}

/**
 * El returnTo CANÓNICO para un guard de servidor, y no la petición cruda.
 *
 * Una entrada anónima por una URL con org (`/rooms/acme/channels/x`) llega al
 * guard REESCRITA por dentro (`/rooms/channels/x?__org=acme`): componer el
 * returnTo con `url.pathname + url.search` mandaría tras el login a la forma
 * con marca, no canónica — la barra quedaría en la forma interna y la
 * convergencia dependería de cada app. Con la marca, el returnTo sale en la
 * forma org-ful canónica (`/rooms/acme/channels/x`); sin marca, tal cual
 * entró (las entradas sin org ya las canariza el middleware tras el login).
 */
export function returnToCanonico(url: URL, base: string): string {
  const org = url.searchParams.get(MARCA_ORG);
  const b = stripTrailingSlashes(base);
  // La consola de plataforma se sirve en la RAÍZ (SYTUE_BASE_PATH vacío) y NUNCA
  // lleva org: con `b === ''` el `startsWith('/')` de abajo daría «de la app»
  // para cualquier URL y una marca artesanal compondría `/org/...` — un 404 tras
  // el login. Tal cual entró, que es lo único que ahí se puede devolver.
  if (!b) return url.pathname + url.search;
  const deLaApp = url.pathname.startsWith(`${b}/`) || url.pathname === b;
  if (!org || !RE_SLUG_ORG.test(org) || !deLaApp) return url.pathname + url.search;
  const destino = new URL(url.origin);
  const ruta = url.pathname.slice(b.length).replace(/^\/+/, '');
  destino.pathname = ruta ? `${b}/${org}/${ruta}` : `${b}/${org}/`;
  destino.search = url.search;
  destino.searchParams.delete(MARCA_ORG);
  return destino.pathname + destino.search;
}
