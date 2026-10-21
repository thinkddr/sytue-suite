import type { DocKind } from '@sytue/api-clients';
import { PRODUCTO_DE_TIPO, appUrl } from '@sytue/config';
import { CATALOGO } from '@sytue/ui';

/**
 * Los cuatro productos de la ofimática: Docus (texto), Calcus (hojas), Diapos
 * (presentaciones) y CertiPDF (PDF). Las claves son las de `ProductKey`.
 *
 * Desde el 2026-10-12 cada uno tiene su app (`apps/docus`, `apps/calcus`,
 * `apps/diapos`, `apps/certipdf`), con su host y su prefijo, y todas montan este
 * paquete (ADR 0076).
 *
 * Los cuatro son de Sytue WorkHub, la suite ofimática, que NO es Sytue Hub.
 */
export type ProductoDocs = 'docs' | 'sheets' | 'slides' | 'pdf';

/**
 * El nombre sale del CATÁLOGO y no de una copia: tres copias de esta tabla ya
 * divergieron una vez, y el renombrado v4 del producto `pdf` habría dejado la
 * pestaña con el nombre viejo. `??` solo por el tipo: las cuatro claves están.
 */
export const NOMBRE: Record<ProductoDocs, string> = {
  docs: CATALOGO.get('docs')?.name ?? 'Sytue Docus',
  sheets: CATALOGO.get('sheets')?.name ?? 'Sytue Calcus',
  slides: CATALOGO.get('slides')?.name ?? 'Sytue Diapos',
  pdf: CATALOGO.get('pdf')?.name ?? 'Sytue CertiPDF',
};

/** El producto que abre cada tipo: la tabla es la de @sytue/config, que usa toda la suite. */
export const PRODUCTO_POR_KIND: Record<DocKind, ProductoDocs> = PRODUCTO_DE_TIPO;

export function esKind(k: string | null | undefined): k is DocKind {
  return k === 'document' || k === 'spreadsheet' || k === 'presentation';
}

/** Producto para un `kind` (de la URL o del documento); sin tipo válido, Docus. */
export function productoDeKind(k: string | null | undefined): ProductoDocs {
  return esKind(k) ? PRODUCTO_POR_KIND[k] : 'docs';
}

/**
 * En cliente, con «color de cada app»: la hoja lee el producto de `data-app` (ADR 0072).
 * El layout ya lo puso en el servidor; esto lo pone al saber el tipo del documento
 * abierto. Los iconos de la pestaña no cambian: cada app sirve solo su producto (ADR 0076).
 */
export function aplicarMarca(p: ProductoDocs): void {
  if (typeof document === 'undefined') return;
  document.documentElement.dataset.app = p;
}

/**
 * El editor del producto de un documento cuando NO es el de esta app; null si lo es,
 * si quien llama no fija el tipo (`app` ausente) o si no se sabe.  Solo viajan el
 * id y la vuelta a Drive: el resto de la query puede ser la marca interna de la org.
 */
export function editorDeOtroProducto(
  app: DocKind | undefined,
  real: DocKind | null,
  { id, volver, org }: { id: string; volver: string | null; org: string | null },
): string | null {
  if (!app || !real || real === app) return null;
  const query = new URLSearchParams({ id });
  if (volver) query.set('volver', volver);
  return appUrl(PRODUCTO_POR_KIND[real], `/edit?${query}`, org);
}
