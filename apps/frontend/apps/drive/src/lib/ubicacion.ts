// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos

/**
 * Ubicación de un recurso: «Mostrar en su carpeta» y abrir una carpeta desde las vistas
 * que cruzan todo el árbol (Recientes, Destacados, resultados de búsqueda).
 *
 * Esas vistas traen la RUTA de cada recurso y no su carpeta, así que la carpeta sale de
 * la ruta. Y el explorador ya sabe aterrizar en una carpeta leyendo `?path=` al montarse
 * (el deep link de la app de escritorio): llevar a alguien a su carpeta es escribir esa
 * URL y montar «Mi unidad». `?select=` dice qué elemento marcar al llegar.
 */

import { withBase } from '@sytue/ui';
import { leerOrden, parametrosDeOrden, type Orden } from './orden';

/** Tamaño de página del listado de una carpeta (el `limit` del BFF). */
export const TAMANO_PAGINA = 200;

/** La carpeta que contiene una ruta: `/a/b.pdf` → `/a`; lo de la raíz, `/`. */
export function carpetaPadre(path: string): string {
  const limpia = path.replace(/\/+$/, '');
  const corte = limpia.lastIndexOf('/');
  return corte <= 0 ? '/' : limpia.slice(0, corte);
}

/**
 * La URL de la página con «Mi unidad» abierta en `carpeta` y, si se da, `select` marcado.
 * Parte de la URL actual para no perder el prefijo de la app ni lo que no es de aquí, y
 * quita lo que mandaría a otro sitio al montar: la sección (`?section=`) y el visor de
 * un deep link (`?preview=`), que no debe dispararse al volver a su carpeta.
 */
export function urlDeUbicacion(href: string, carpeta: string, select?: string): string {
  const u = new URL(href);
  if (carpeta === '/' || carpeta === '') u.searchParams.delete('path');
  else u.searchParams.set('path', carpeta);
  if (select) u.searchParams.set('select', select);
  else u.searchParams.delete('select');
  u.searchParams.delete('preview');
  u.searchParams.delete('section');
  return u.toString();
}

/**
 * La petición del listado de una carpeta. Vive aquí, y la usa `drive.list`, porque es
 * también la clave con la que el service worker la guarda: si las dos se escribieran por
 * separado, la comprobación de abajo dejaría de encontrar nada sin que fallara ningún test.
 */
export function urlDeListado(path: string, cursor?: string, orden?: Orden): string {
  const p = orden ? parametrosDeOrden(orden) : null;
  // El orden va en la URL también con cursor: el cursor lleva su orden y el servicio
  // rechaza (400) seguirlo con otro.
  const qs =
    `path=${encodeURIComponent(path)}&limit=${TAMANO_PAGINA}` +
    (p ? `&orden=${p.orden}&sentido=${p.sentido}` : '');
  // Con el prefijo de la app (`/drive`): el service worker guarda la petición con la URL
  // que salió de verdad, que el parche de `fetch` ya prefija. Sin él, la clave de la
  // caché era la raíz del host y nunca casaba: sin red la opción no salía jamás.
  // `withBase` es idempotente, así que en `fetch` no se prefija dos veces.
  return withBase(
    import.meta.env.BASE_URL,
    `/api/files?${qs}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`,
  );
}

/**
 * Si la primera página de `carpeta` está guardada para verla sin conexión, en el orden
 * que se recuerda, que es el que pedirá «Mi unidad» al llegar. Sin red,
 * «Mostrar en su carpeta» solo se ofrece entonces: llevar a una carpeta que no se puede
 * pintar sería un menú que promete y luego enseña un error. Sin Cache Storage (contexto
 * inseguro, navegador sin él) se contesta que no, que es lo prudente.
 */
export async function carpetaEnCache(
  carpeta: string,
  almacen: Pick<CacheStorage, 'match'> | undefined = typeof caches === 'undefined'
    ? undefined
    : caches,
  base: string = typeof location === 'undefined' ? 'http://localhost/' : location.href,
): Promise<boolean> {
  if (!almacen) return false;
  try {
    const url = new URL(urlDeListado(carpeta, undefined, leerOrden()), base).toString();
    return (await almacen.match(url)) !== undefined;
  } catch {
    return false;
  }
}

/** Lo que hay que marcar cuando llegue su carpeta: la de «Mostrar en su carpeta». */
export interface Pendiente {
  carpeta: string;
  ruta: string;
}

/** El `?select=` con el que se llega, con su carpeta (`?path=`); null si no hay. */
export function pendienteDeLaUrl(search: string): Pendiente | null {
  const qs = new URLSearchParams(search);
  const ruta = qs.get('select') ?? '';
  return ruta ? { carpeta: qs.get('path') || '/', ruta } : null;
}

/**
 * Qué hacer con lo pendiente al pintar `items` de `abierta`:
 *   - `olvidar` si ya se mira otra carpeta (quien mira ha navegado, y marcar algo al
 *     volver sería una sorpresa);
 *   - `esperar` si aún no hay lista, o la que hay no es la de su carpeta (la de la
 *     carpeta anterior sigue en pantalla hasta que llega la nueva, en el mismo pintado
 *     en que cambia la ruta);
 *   - `falta` si la lista ya es la de su carpeta y no está en ella: puede estar en una
 *     página que aún no ha llegado (el listado pagina de `TAMANO_PAGINA` en
 *     `TAMANO_PAGINA`), o ya no existir;
 *   - el elemento y su posición en `items`, si está.
 */
export function aMarcar<T extends { path: string }>(
  items: readonly T[] | null,
  pendiente: Pendiente,
  abierta: string,
): 'olvidar' | 'esperar' | 'falta' | { item: T; indice: number } {
  if (abierta !== pendiente.carpeta) return 'olvidar';
  if (!items) return 'esperar';
  const indice = items.findIndex((i) => i.path === pendiente.ruta);
  if (indice >= 0) return { item: items[indice]!, indice };
  // Una carpeta vacía no dice de quién es: se espera, y no hay nada que marcar en ella.
  const deSuCarpeta =
    items.length > 0 && items.every((i) => carpetaPadre(i.path) === pendiente.carpeta);
  return deSuCarpeta ? 'falta' : 'esperar';
}
