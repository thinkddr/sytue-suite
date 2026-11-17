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
 * la MISMA URL con una página más (`?paginas=2`): se pide al Hub, en UNA búsqueda
 * federada, todo lo que se enseña (`limit = páginas × POR_PAGINA`, desde el principio).
 * Antes era una petición por página —el Hub acotaba `limit` a su máximo POR ORIGEN—, y
 * cada clic volvía a lanzar todas: con `?paginas=10`, unas cien llamadas a servicios.
 */
export const POR_PAGINA = 20;
/**
 * Tope de páginas: el tamaño de página máximo del agregado en el Hub, 10 × `SEARCH_LIMIT`
 * (20) = 200. Pasado el tope el Hub recortaría y el botón no traería nada nuevo.
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

/** Lo que `buscarPaginas` necesita del cliente del Hub (el SDK lo cumple). */
export interface BuscadorDelHub {
  search: (
    query: string,
    sources: undefined,
    page: { limit: number; offset: number },
  ) => Promise<Paginas>;
}

/**
 * Las `paginas` primeras páginas del agregado en UNA sola petición al Hub. El ranking
 * es el de una sola búsqueda, así que no hay repetidos que quitar ni huecos entre
 * páginas, y el aviso de orígenes caídos es el de esa búsqueda.
 *
 * Si falla un «Cargar más» (`paginas` ≥ 2) se piden las que ya se veían: el fallo NO
 * tira lo de antes —es lo que hace Fyne— y `cortada` deja el botón para reintentar.
 * Solo si falla la primera página la búsqueda entera ha fallado (lanza).
 */
export async function buscarPaginas(
  hub: BuscadorDelHub,
  q: string,
  paginas: number,
): Promise<{ res: Paginas; paginas: number; cortada: boolean }> {
  const pedir = (n: number) => hub.search(q, undefined, { limit: n * POR_PAGINA, offset: 0 });
  try {
    return { res: await pedir(paginas), paginas, cortada: false };
  } catch (err) {
    if (paginas < 2) throw err;
    return { res: await pedir(paginas - 1), paginas: paginas - 1, cortada: true };
  }
}

/**
 * Hay «Cargar más» si el Hub dice que quedan y no se ha llegado al tope. Manda el
 * `has_more`, que el Hub calcula sobre el agregado. El respaldo por total compara con lo
 * PEDIDO (`paginas × POR_PAGINA`), no con lo enseñado.
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
 * lo enseñado ES todo: si los orígenes cambian entre el recuento y la página, «39 de
 * 40» sin botón prometería un resultado que no existe. Con el tope de páginas alcanzado
 * y más en el Hub, el total del Hub sigue siendo la verdad.
 */
export function totalAMostrar(p: Paginas, paginas: number): number {
  return quedanEnElHub(p, paginas) ? p.total : p.results.length;
}

/**
 * El último «Cargar más» no trajo nada: la petición falló (`cortada`) o la página pedida
 * llegó sin un solo resultado más de los que ya se veían (vacía aunque el Hub dijera
 * `has_more`). La pantalla lo dice junto al botón; si no, se recargaría igual y arriba
 * del todo.
 */
export function cargaSinNovedad(b: { res: Paginas; paginas: number; cortada: boolean }): boolean {
  if (b.cortada) return true;
  return b.paginas >= 2 && b.res.results.length <= (b.paginas - 1) * POR_PAGINA;
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
