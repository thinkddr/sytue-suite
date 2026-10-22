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
