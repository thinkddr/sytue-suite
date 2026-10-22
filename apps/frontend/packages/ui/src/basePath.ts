/**
 * El prefijo bajo el que se sirve cada app, aplicado a lo que el navegador pide.
 *
 * Las nueve apps se construyen con `base` (`/admin`, `/one`, `/drive`…) porque
 * las dos direcciones son válidas: `admin.sytue.eu/admin` y también
 * `<organizacion>.sytue.eu/admin`. Astro prefija sus propios assets y las rutas
 * de sus páginas, pero NO lo que escribimos a mano: un `fetch('/api/...')`
 * dentro de un componente pide la raíz del host, donde el ingress no enruta
 * nada. En producción eso son 184 llamadas que devuelven el 404 de Traefik —la
 * mitad de la interfaz «no carga» sin un solo error en los logs—.
 *
 * Este script normaliza esas rutas en UN punto: envuelve `fetch` para prefijar
 * las que empiezan por `/api/` o `/auth/`, que son las nuestras. Es la misma
 * decisión que una guarda en la función compartida en vez de en cada llamante:
 * arreglar las 184 una a una deja fuera la 185 que alguien escriba mañana.
 *
 * ponytail: solo `/api/` y `/auth/`. No se toca ninguna otra ruta absoluta
 * —`/favicon.svg`, un enlace a otra app— porque el shim no puede saber si son de
 * esta app o del host. Los `href` de navegación se prefijan en su sitio, con
 * `withBase`. Si algún día las apps se sirven en la raíz, esto se queda en no-op
 * solo (base vacío ⇒ no envuelve nada).
 *
 * El script publica además `window.__sytueBase`, que es lo que hace que `withBase`
 * acierte cuando el prefijo del HOST no es el del build: ver su comentario.
 */
export function basePathScript(base: string): string {
  const b = (base || '/').replace(/\/+$/, '');
  // El valor se publica SIEMPRE, también cuando está vacío: es justo el caso de
  // `platform.<dominio>` —servido en la raíz— y sin publicarlo `withBase` caería
  // al `/admin` del build. Que la cadena vacía sea significativa es la razón de
  // que se distinga «publicado y vacío» de «no publicado».
  const publicar = `window.__sytueBase=${JSON.stringify(b)};`;
  if (!b) return publicar;
  return (
    publicar +
    `(function(){var b=${JSON.stringify(b)};var f=window.fetch;` +
    `window.fetch=function(input,init){` +
    // Solo el caso string: un Request ya trae la URL absoluta resuelta por el
    // navegador y reescribirlo exigiría reconstruirlo entero.
    `if(typeof input==='string'&&(input.indexOf('/api/')===0||input.indexOf('/auth/')===0)){` +
    `input=b+input;}` +
    `return f.call(this,input,init);};})();`
  );
}

/**
 * El registro del Service Worker, con el prefijo de la app.
 *
 * Va aquí y no suelto en cada layout porque estaba triplicado y las tres copias
 * pedían `/sw.js`: la raíz del host, donde no hay nada. En producción eso son
 * tres 404 permanentes —portal, rooms y Cloud— y, peor, **ninguna de las tres
 * PWA funciona**: sin worker no hay caché offline ni `offline.html`.
 *
 * No se veía en ningún sitio. El `.catch(() => {})` es correcto —un fallo del
 * worker no debe tumbar la app— pero también se traga este error, así que el
 * único rastro era la petición roja en la pestaña de red. Y el shim de `fetch`
 * no llega: `register()` no pasa por `fetch`.
 *
 * El scope sale del propio fichero: registrar `/one/sw.js` acota el worker a
 * `/one/`, que es exactamente lo que esta app debe controlar.
 */
export function swRegisterScript(base: string): string {
  const b = (base || '/').replace(/\/+$/, '');
  const url = JSON.stringify(`${b}/sw.js`);
  return (
    `if('serviceWorker' in navigator){window.addEventListener('load',function(){` +
    `navigator.serviceWorker.register(${url}).catch(function(){});});}`
  );
}

/**
 * Prefija una ruta de ESTA app. Para el código que construye URL a mano fuera
 * del `fetch` —`location.assign`, un `href`—, donde el shim no llega.
 *
 * `base` viene de `import.meta.env.BASE_URL` de la app (Vite lo sustituye al
 * construir). Una ruta que ya empieza por el prefijo se deja intacta, para que
 * aplicarlo dos veces no dé `/admin/admin/...`.
 */
/**
 * Sufijo de caché de los tres iconos del `<head>` (`favicon.ico`, `favicon.svg`
 * y `apple-touch-icon.png`). Safari guarda el icono POR PÁGINA y no vuelve a
 * pedirlo aunque el `<link>` cambie: quien abrió Mail cuando el icono daba 404
 * seguía sin icono días después del arreglo, mientras Calendar —abierto por
 * primera vez ya arreglado— sí lo tenía. Medido el 2026-09-09 en producción:
 * mismo HTML, mismos bytes y mismo 200 en las seis apps. Cambiar la URL es lo
 * único que hace a Safari volver a mirar. Súbelo cada vez que cambie un icono.
 *
 * A v6 el 2026-09-14: ERP adoptó el color ámbar (#b45309) y su edificio sólido.
 * Subir a v6 invalida la caché persistente de Safari para que cargue el favicon
 * naranja y coincida con la marca del Hub y del selector de aplicaciones.
 *
 * A v7 el 2026-09-15: se aclararon Rooms, Mail, Calendar, Docus, Diapos y PDF. En
 * modo oscuro Safari pinta una placa clara detrás del favicon cuya luminancia media
 * a 16 px no llega a ~0,29 (medido: con borde ≤ 0,283, sin borde ≥ 0,288); ahora
 * todos pasan de 0,30. Si se oscurece una baldosa, vuelve el borde.
 */
export const SUFIJO_ICONOS = '?v=7';

export function withBase(base: string, path: string): string {
  // El prefijo del HOST manda sobre el del build cuando lo hay. La consola se
  // sirve bajo `/admin` en `admin.<dominio>` y en la RAÍZ en `platform.<dominio>`
  // —misma imagen, dos superficies—, pero `import.meta.env.BASE_URL` lo sustituye
  // Vite al construir, así que vale `/admin` en las dos y en plataforma mandaba a
  // `/admin/...`, que ahí es un 404 medido. Lo publica `basePathScript` con el
  // valor que el servidor conoce (`SYTUE_BASE_PATH`).
  const delHost =
    typeof window !== 'undefined' ? (window as { __sytueBase?: string }).__sytueBase : undefined;
  const b = (delHost ?? base ?? '/').replace(/\/+$/, '');
  if (!b || !path.startsWith('/')) return path;
  return path === b || path.startsWith(`${b}/`) ? path : `${b}${path}`;
}
