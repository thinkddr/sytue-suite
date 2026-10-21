/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Notas al final (endnotes) — AYUDAS PURAS (sin DOM). Hermanas de las notas al
 * pie, pero recopiladas al FINAL del documento (no por página) y numeradas en
 * ROMANO (i, ii, iii…) para distinguirlas de las notas al pie (arábigo 1, 2, 3).
 * El pegamento con el folio (insertar el marcador, mantener la <section
 * data-endnotes>, renumerar al insertar/borrar) vive en ribbon/tabs/Referencias.tsx;
 * aquí solo la lógica no trivial y testeable: convertir a número romano y
 * RENUMERAR/RECOPILAR las notas en orden de documento. Mismo patrón que
 * bibliography.ts / formfields.ts. Ejecutar el self-test:
 *   cd apps/frontend && bun test packages/office/test/endnotes.test.ts
 */

/** Símbolos romanos de mayor a menor (incluye los sustractivos cd/xl/ix…). */
const ROMAN: readonly [number, string][] = [
  [1000, 'm'],
  [900, 'cm'],
  [500, 'd'],
  [400, 'cd'],
  [100, 'c'],
  [90, 'xc'],
  [50, 'l'],
  [40, 'xl'],
  [10, 'x'],
  [9, 'ix'],
  [5, 'v'],
  [4, 'iv'],
  [1, 'i'],
];

/**
 * Número romano en minúscula (i, ii, iii, iv…). Clamp a ≥ 1 (mismo criterio que
 * el resto de ayudas puras): un índice inválido cae en «i». Algoritmo general
 * (vale para cualquier N), aunque las notas nunca lleguen a valores altos.
 */
export function romanNumeral(n: number): string {
  let x = Math.max(1, Math.floor(n) || 1);
  let out = '';
  for (const [value, symbol] of ROMAN) {
    while (x >= value) {
      out += symbol;
      x -= value;
    }
  }
  return out;
}

/** Una nota al final ya renumerada, lista para pintar el marcador y el <li>. */
export interface EndnoteEntry {
  /** Secuencia 1..N en orden de documento (nuevo id estable enref-N/en-N). */
  seq: number;
  /** Etiqueta romana visible (= romanNumeral(seq)). */
  label: string;
  /** HTML de la nota preservado; '' si el marcador no tiene nota (se pondrá un
   *  texto provisional al pintar). */
  content: string;
}

/**
 * RENUMERA y RECOPILA las notas al final en ORDEN DE DOCUMENTO: dada la lista de
 * ids de referencia de los marcadores (en el orden en que aparecen en el cuerpo)
 * y el contenido de cada nota por id, devuelve una entrada por marcador con su
 * secuencia 1..N, su etiqueta romana y el contenido de su nota.
 *
 * - El ORDEN lo fija la aparición del marcador en el cuerpo (no el de las notas).
 * - Un marcador sin nota (id ausente en el mapa) → content ''.
 * - Una nota HUÉRFANA (en el mapa pero sin marcador que la referencie) se
 *   DESCARTA (no se itera): al borrar el marcador, su nota desaparece.
 * - Un id DUPLICADO (marcador copiado y pegado) produce una entrada por
 *   aparición, todas con el mismo contenido de origen (a partir de ahí divergen).
 *
 * Pura: la cubre el self-test. El .tsx la empareja por índice con los elementos
 * <sup> reales y reconstruye la sección.
 */
export function renumberEndnotes(
  orderedRefIds: readonly string[],
  contentById: ReadonlyMap<string, string>,
): EndnoteEntry[] {
  return orderedRefIds.map((id, i) => ({
    seq: i + 1,
    label: romanNumeral(i + 1),
    content: contentById.get(id) ?? '',
  }));
}
