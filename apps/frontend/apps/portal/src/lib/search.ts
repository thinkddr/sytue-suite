/**
 * Agrupación de resultados de la búsqueda federada (Sytue Hub) por producto.
 * Función pura, sin dependencias de runtime — se prueba en test/search.test.ts.
 */
import type { SearchResult } from '@sytue/api-clients';
import type { ProductKey } from '@sytue/config';

export interface SourceGroup {
  key: ProductKey;
  items: SearchResult[];
}

/**
 * Agrupa por producto preservando el orden global de aparición: el Hub ya
 * rankea los resultados, así que el primer producto que aparece encabeza la
 * lista y el orden dentro de cada grupo respeta ese ranking.
 */
export function groupBySource(results: SearchResult[]): SourceGroup[] {
  const order: ProductKey[] = [];
  const buckets = new Map<ProductKey, SearchResult[]>();
  for (const r of results) {
    let bucket = buckets.get(r.source);
    if (!bucket) {
      bucket = [];
      buckets.set(r.source, bucket);
      order.push(r.source);
    }
    bucket.push(r);
  }
  return order.map((key) => ({ key, items: buckets.get(key) ?? [] }));
}

/**
 * «Cargar más» de la búsqueda. La página es SSR y sin JS de cliente, así que «más» es
 * la MISMA URL con una página más (`?paginas=2`): se piden al Hub las páginas una a
 * una por `offset` —el Hub acota `limit` a su máximo por petición, así que pedir 40
 * de golpe devuelve 20— y se pegan en orden.
 */
export const POR_PAGINA = 20;
/**
 * Tope de páginas: lo que el Hub puede devolver, `SEARCH_LIMIT` (20) por origen × sus diez
 * orígenes = 200. Cada «más» vuelve a pedir TODAS las páginas (limitación conocida, en
 * docs/roadmap.md «Hub + Portal + www»): el tope acota también lo que cuesta un `?paginas=`.
 */
export const MAX_PAGINAS = 10;

/** `?paginas=` leído como dato no confiable: 1..MAX_PAGINAS, 1 si no vale. */
export function paginasPedidas(raw: string | null): number {
  const n = Number.parseInt(raw ?? '', 10);
  if (!Number.isFinite(n) || n < 1) return 1;
  return Math.min(n, MAX_PAGINAS);
}

export interface Paginas {
  results: SearchResult[];
  total: number;
  hasMore: boolean;
  fallidos: { source: ProductKey; error: string }[];
}

/**
 * Une las páginas en el orden en que se pidieron. Cada una es una búsqueda aparte, así
 * que entre dos el ranking puede moverse: un resultado repetido sale una vez, en su
 * primera posición. Un origen que falla en cualquiera de ellas se avisa una vez.
 */
export function unirPaginas(paginas: readonly Paginas[]): Paginas {
  const vistos = new Set<string>();
  const results: SearchResult[] = [];
  const fallidos = new Map<ProductKey, { source: ProductKey; error: string }>();
  let total = 0;
  for (const p of paginas) {
    for (const r of p.results) {
      const clave = `${r.source}\u0000${r.id}`;
      if (vistos.has(clave)) continue;
      vistos.add(clave);
      results.push(r);
    }
    for (const f of p.fallidos) if (!fallidos.has(f.source)) fallidos.set(f.source, f);
    total = Math.max(total, p.total);
  }
  const ultima = paginas.at(-1);
  return {
    results,
    total: Math.max(total, results.length),
    hasMore: ultima?.hasMore === true,
    fallidos: [...fallidos.values()],
  };
}

/**
 * Las páginas que se pueden enseñar de las pedidas en paralelo: las seguidas desde la
 * primera hasta el primer fallo. Una página que falla corta ahí —lo de detrás dejaría un
 * hueco en el ranking— pero NO tira lo anterior: es lo que hace Fyne cuando falla un
 * «Cargar más», y el botón queda para reintentar. Solo si falla la primera (`null`) la
 * búsqueda entera ha fallado.
 */
export function paginasHastaElFallo(
  pedidas: readonly PromiseSettledResult<Paginas>[],
): { paginas: Paginas[]; cortada: boolean } | null {
  const paginas: Paginas[] = [];
  for (const p of pedidas) {
    if (p.status === 'rejected') break;
    paginas.push(p.value);
  }
  if (paginas.length === 0) return null;
  return { paginas, cortada: paginas.length < pedidas.length };
}

/**
 * Hay «Cargar más» si el Hub dice que quedan y no se ha llegado al tope. Manda el
 * `has_more` de la ÚLTIMA página, que el Hub calcula sobre el agregado. El respaldo por
 * total compara con lo PEDIDO (`paginas × POR_PAGINA`), no con lo enseñado: tras quitar
 * repetidos lo enseñado queda por debajo del total aunque no quede nada, y el botón no
 * se iría nunca.
 */
export function hayMas(p: Paginas, paginas: number): boolean {
  return paginas < MAX_PAGINAS && quedanEnElHub(p, paginas);
}

/** El Hub tiene más de lo pedido: su `has_more`, o un total por encima de lo pedido. */
function quedanEnElHub(p: Paginas, paginas: number): boolean {
  return p.hasMore || p.total > paginas * POR_PAGINA;
}

/**
 * El total que se puede decir en «Se muestran 39 de 40». Si el Hub ya no tiene nada más,
 * lo enseñado ES todo: tras quitar un repetido, «39 de 40» sin botón prometería un
 * resultado que no existe. Con el tope de páginas alcanzado y más en el Hub, el total
 * del Hub sigue siendo la verdad.
 */
export function totalAMostrar(p: Paginas, paginas: number): number {
  return quedanEnElHub(p, paginas) ? p.total : p.results.length;
}

/**
 * El último «Cargar más» no trajo nada: una página pedida falló (`cortada`) o llegó sin
 * un solo resultado nuevo (vacía aunque el Hub dijera `has_more`, o todo repetidos). La
 * pantalla lo dice junto al botón; si no, se recargaría igual y arriba del todo.
 */
export function cargaSinNovedad(hasta: { paginas: readonly Paginas[]; cortada: boolean }): boolean {
  if (hasta.cortada) return true;
  if (hasta.paginas.length < 2) return false;
  const antes = unirPaginas(hasta.paginas.slice(0, -1)).results.length;
  return unirPaginas(hasta.paginas).results.length === antes;
}

/** El `id` de la fila del resultado `n` (desde 0): el ancla donde aterriza «Cargar más». */
export function idResultado(n: number): string {
  return `r-${n}`;
}

/**
 * La URL de «Cargar más»: la misma consulta con una página más, y un ancla al primer
 * resultado nuevo (`mostrados` es cuántos se ven ya) para que la navegación aterrice
 * ahí y no arriba del todo. `ruta` pone el prefijo de la app y la organización (`r()` de
 * `lib/rutas.ts`): un `/buscar` a pelo pide la raíz del host, donde el ingress no enruta
 * nada. Es obligatorio para que no se pueda olvidar.
 */
export function enlaceCargarMas(
  ruta: (path: string) => string,
  q: string,
  paginas: number,
  mostrados: number,
): string {
  const query = new URLSearchParams({ q, paginas: String(paginas + 1) });
  return `${ruta(`/buscar?${query}`)}#${idResultado(mostrados)}`;
}
