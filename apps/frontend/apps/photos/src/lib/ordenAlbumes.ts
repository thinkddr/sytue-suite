/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * En qué orden se ven las tarjetas de «Álbumes». El servicio los da por última
 * modificación (`updated_at DESC`), que es el orden «recientes» y el de siempre;
 * los otros dos son de lectura y no tocan nada en el servidor.
 *
 * Fyne aplica la MISMA regla en `apps/fyne/apps/photos/orden_albumes.go`, con los
 * mismos casos en `testdata/orden-albumes.json`.
 */
import { intlTag, type Locale } from '@sytue/i18n';

export const ORDENES_ALBUM = ['recent', 'title', 'count'] as const;
export type OrdenAlbum = (typeof ORDENES_ALBUM)[number];

/** La clave de `localStorage`, la misma que guarda Fyne en sus preferencias. */
export const CLAVE_ORDEN_ALBUMES = 'sytue.photos.ordenAlbumes';

export function esOrdenAlbum(v: string | null | undefined): v is OrdenAlbum {
  return typeof v === 'string' && (ORDENES_ALBUM as readonly string[]).includes(v);
}

interface AlbumOrdenable {
  id: string;
  title: string;
  count: number;
  updatedAt: string;
}

/**
 * Ordena una copia. Cada orden desempata con el siguiente criterio que tiene sentido
 * y, al final, con el id: dos álbumes iguales no pueden bailar entre recargas.
 */
export function ordenarAlbumes<A extends AlbumOrdenable>(
  lista: readonly A[],
  orden: OrdenAlbum,
  locale: Locale,
): A[] {
  const col = new Intl.Collator(intlTag(locale), { sensitivity: 'base', numeric: true });
  const porTitulo = (a: A, b: A) => col.compare(a.title.trim(), b.title.trim());
  // Por instante y no como texto: el servicio manda `time.Time` en RFC 3339 con los
  // decimales recortados («…10:00:00Z» frente a «…10:00:00.25Z») y con la zona de
  // su reloj, así que el texto no ordena como el tiempo. Sin fecha, el más antiguo,
  // como el `time.Time{}` de Fyne.
  const porReciente = (a: A, b: A) => {
    const x = instante(a.updatedAt);
    const y = instante(b.updatedAt);
    return x < y ? 1 : x > y ? -1 : 0;
  };
  const porId = (a: A, b: A) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
  const cmp: Record<OrdenAlbum, (a: A, b: A) => number> = {
    recent: (a, b) => porReciente(a, b) || porId(b, a),
    title: (a, b) => porTitulo(a, b) || porReciente(a, b) || porId(a, b),
    count: (a, b) => b.count - a.count || porTitulo(a, b) || porId(a, b),
  };
  return [...lista].sort(cmp[orden]);
}

function instante(iso: string | undefined): number {
  const ms = iso ? Date.parse(iso) : Number.NaN;
  return Number.isNaN(ms) ? Number.NEGATIVE_INFINITY : ms;
}

/** El orden guardado en este navegador; sin almacenamiento (o bloqueado), «recientes». */
export function leerOrdenAlbumes(): OrdenAlbum {
  try {
    const v =
      typeof localStorage === 'undefined' ? null : localStorage.getItem(CLAVE_ORDEN_ALBUMES);
    return esOrdenAlbum(v) ? v : 'recent';
  } catch {
    return 'recent';
  }
}

export function guardarOrdenAlbumes(o: OrdenAlbum): void {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(CLAVE_ORDEN_ALBUMES, o);
  } catch {
    // Un navegador sin almacenamiento sigue ordenando; solo no lo recuerda.
  }
}
