/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Autosuma» (Σ en Inicio y Alt+=): la fórmula que se escribe sola al pie de una columna
 * de números, que es lo primero que busca quien viene de Excel o de Sheets. Sin ella había
 * que teclear `=SUM(` y arrastrar el rango a mano, que es justo el paso en el que se
 * equivoca una fila.
 *
 * Aquí vive solo el PLAN —qué celdas reciben fórmula y sobre qué rango—, puro y sin
 * modelo: quien llama dice qué celda tiene un número y cuál está vacía. La app nativa
 * tiene su gemelo en `apps/fyne/calcus/autosuma.go`, y los dos recorren los mismos
 * vectores (`test/vectores/calcus-herramientas.json`): la misma selección no puede
 * sumar otro rango según dónde se pulse.
 *
 * Las reglas son las de Excel, recortadas a lo que se explica en una frase:
 *  - Una celda sola: los números seguidos que tiene ENCIMA; si no hay, los de su
 *    IZQUIERDA. Sin ninguno no hay plan (`null`) y quien llama abre la celda con `=SUM(`
 *    para que se elija el rango a mano.
 *  - Varias filas: un total por columna con algún número, en la última fila de la
 *    selección si está vacía entera, y si no en la de debajo.
 *  - Una sola fila: un total a la derecha, con el mismo criterio de la última columna.
 */
import { numToCol } from '@sytue/formula/ligero';
import type { RectHoja } from './atajosHoja';

/** Las funciones del desplegable de Σ, en el orden de Excel. */
export const FUNCIONES_AUTOSUMA = ['SUM', 'AVERAGE', 'COUNT', 'MAX', 'MIN'] as const;
export type FuncionAutosuma = (typeof FUNCIONES_AUTOSUMA)[number];

export function esFuncionAutosuma(v: unknown): v is FuncionAutosuma {
  return typeof v === 'string' && (FUNCIONES_AUTOSUMA as readonly string[]).includes(v);
}

/** Lo que el plan necesita saber de la hoja. */
export interface LectorAutosuma {
  /** La celda calcula un número (un literal numérico o una fórmula que da número). */
  esNumero: (c: number, r: number) => boolean;
  /** La celda no tiene nada. */
  vacia: (c: number, r: number) => boolean;
}

/** Una fórmula del plan: dónde se escribe y qué rango suma. */
export interface TotalAutosuma {
  c: number;
  r: number;
  rango: RectHoja;
}

/**
 * El plan de Σ para la selección `sel`. `null` = celda sola sin números al lado (se abre
 * con `=SUM(`); `[]` = un rango sin ningún número, o sin sitio para el total (la última
 * fila o columna de la hoja): no hay nada que escribir.
 */
export function planDeAutosuma(
  sel: RectHoja,
  hoja: LectorAutosuma,
  topes: { cols: number; filas: number },
): TotalAutosuma[] | null {
  const { c1, c2, r1, r2 } = sel;
  if (c1 === c2 && r1 === r2) {
    let k = r1 - 1;
    while (k >= 0 && hoja.esNumero(c1, k)) k--;
    if (k < r1 - 1) return [{ c: c1, r: r1, rango: { c1, c2: c1, r1: k + 1, r2: r1 - 1 } }];
    k = c1 - 1;
    while (k >= 0 && hoja.esNumero(k, r1)) k--;
    if (k < c1 - 1) return [{ c: c1, r: r1, rango: { c1: k + 1, c2: c1 - 1, r1, r2: r1 } }];
    return null;
  }
  if (r2 > r1) {
    let ultimaVacia = true;
    for (let c = c1; c <= c2 && ultimaVacia; c++) ultimaVacia = hoja.vacia(c, r2);
    const destino = ultimaVacia ? r2 : r2 + 1;
    const hasta = ultimaVacia ? r2 - 1 : r2;
    if (destino >= topes.filas) return [];
    const out: TotalAutosuma[] = [];
    for (let c = c1; c <= c2; c++) {
      let hay = false;
      for (let r = r1; r <= hasta && !hay; r++) hay = hoja.esNumero(c, r);
      if (hay) out.push({ c, r: destino, rango: { c1: c, c2: c, r1, r2: hasta } });
    }
    return out;
  }
  const ultimaVacia = hoja.vacia(c2, r1);
  const destino = ultimaVacia ? c2 : c2 + 1;
  const hasta = ultimaVacia ? c2 - 1 : c2;
  if (destino >= topes.cols) return [];
  for (let c = c1; c <= hasta; c++) {
    if (hoja.esNumero(c, r1)) return [{ c: destino, r: r1, rango: { c1, c2: hasta, r1, r2: r1 } }];
  }
  return [];
}

const ref = (c: number, r: number): string => `${numToCol(c)}${r + 1}`;

/** «=SUM(B2:B9)» (o «=SUM(B2)» si el rango es una celda). */
export function formulaDeAutosuma(fn: FuncionAutosuma, rango: RectHoja): string {
  const desde = ref(rango.c1, rango.r1);
  const hasta = ref(rango.c2, rango.r2);
  return `=${fn}(${desde === hasta ? desde : `${desde}:${hasta}`})`;
}
