/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tabla de datos (análisis de sensibilidad) del Análisis Y-si: helpers PUROS y
 * testeables, sin DOM ni motor de fórmulas. Geometría (paridad Excel):
 *  - UNA variable, valores en la COLUMNA izquierda: la fórmula va a la derecha de
 *    la esquina (fila superior) y los resultados bajo ella; cada valor se fija en
 *    la «celda de entrada de columna».
 *  - UNA variable, valores en la FILA superior: la fórmula va bajo la esquina
 *    (columna izquierda) y los resultados a su derecha; cada valor se fija en la
 *    «celda de entrada de fila».
 *  - DOS variables: la fórmula en la esquina; la fila superior alimenta la entrada
 *    de fila, la columna izquierda la de columna, y la matriz interior se rellena
 *    fijando AMBAS entradas.
 * `planDataTable` calcula la geometría (la parte con ramas y aristas); el lienzo
 * fija las entradas en un motor DESECHABLE y evalúa la fórmula (como la sonda de
 * Buscar objetivo inyecta `f`), y `computeDataTable` la recorre. Una sola fórmula
 * por tabla (Excel admite varias en las de 1 variable; ponytail: ampliar si se pide).
 * Self-test: packages/office/test/sheets-datatable.test.ts (bun test).
 */
import type { CellValue } from '@sytue/formula';
import type { RangeRect } from './format';

/** Tope de celdas de resultado (no bloquear la UI; la rejilla 26×100 ya acota,
 *  esto es una salvaguarda del helper puro, que no conoce el tamaño de la hoja).
 *  ponytail: motor reutilizado → recálculo incremental por celda; subir si hiciera falta. */
export const DATA_TABLE_MAX_CELLS = 5000;

/** Orientación deducida de qué celdas de entrada se han indicado. */
export type DataTableOrientation = 'row' | 'col' | 'both';

/** Motivo por el que no se puede materializar la tabla de datos. */
export type DataTableError = 'no-input' | 'too-small' | 'too-many';

/** Coordenada de celda 0-based. */
interface CR {
  c: number;
  r: number;
}

/** Una celda de resultado del plan: dónde escribir y de qué celdas cabecera
 *  sale el crudo que se fija en cada entrada. */
export interface DataTablePlanCell extends CR {
  /** Celda de la fila superior cuyo crudo se fija en la entrada de FILA (si aplica). */
  rowFrom?: CR;
  /** Celda de la columna izquierda cuyo crudo se fija en la entrada de COLUMNA (si aplica). */
  colFrom?: CR;
}

export interface DataTablePlan {
  orientation: DataTableOrientation;
  /** Celda de la fórmula a evaluar (única). */
  formula: CR;
  /** Celdas de resultado a rellenar, en orden de lectura (fila, luego columna). */
  cells: DataTablePlanCell[];
}

/**
 * Calcula la geometría de una tabla de datos sobre `rect` (0-based, inclusivo)
 * según qué celdas de entrada haya. PURA. Devuelve el plan o el motivo del fallo:
 *  - 'no-input': no se indicó ninguna celda de entrada.
 *  - 'too-small': el rango no basta para la orientación (hace falta ≥ 2×2, con
 *    una franja para valores y otra para la fórmula/resultados).
 *  - 'too-many': el número de resultados supera `maxCells`.
 */
export const planDataTable = (
  rect: RangeRect,
  hasRowInput: boolean,
  hasColInput: boolean,
  maxCells = DATA_TABLE_MAX_CELLS,
): DataTablePlan | { error: DataTableError } => {
  const { c1, r1, c2, r2 } = rect;
  if (!hasRowInput && !hasColInput) return { error: 'no-input' };
  // Todas las orientaciones necesitan una franja de valores y otra de resultados.
  if (c2 - c1 < 1 || r2 - r1 < 1) return { error: 'too-small' };

  const cells: DataTablePlanCell[] = [];
  let orientation: DataTableOrientation;
  let formula: CR;

  if (hasRowInput && hasColInput) {
    orientation = 'both';
    formula = { c: c1, r: r1 };
    for (let r = r1 + 1; r <= r2; r++) {
      for (let c = c1 + 1; c <= c2; c++) {
        cells.push({ c, r, rowFrom: { c, r: r1 }, colFrom: { c: c1, r } });
      }
    }
  } else if (hasColInput) {
    // Valores en la columna izquierda; fórmula/resultados en la columna contigua.
    orientation = 'col';
    formula = { c: c1 + 1, r: r1 };
    for (let r = r1 + 1; r <= r2; r++) cells.push({ c: c1 + 1, r, colFrom: { c: c1, r } });
  } else {
    // Valores en la fila superior; fórmula/resultados en la fila contigua.
    orientation = 'row';
    formula = { c: c1, r: r1 + 1 };
    for (let c = c1 + 1; c <= c2; c++) cells.push({ c, r: r1 + 1, rowFrom: { c, r: r1 } });
  }

  if (cells.length > maxCells) return { error: 'too-many' };
  return { orientation, formula, cells };
};

/** Valor calculado de una celda de resultado. */
export interface DataTableResultCell extends CR {
  value: CellValue;
}

/** Sustituciones a fijar antes de evaluar la fórmula (crudo de la celda cabecera). */
export interface DataTableSubs {
  /** Crudo a fijar en la celda de entrada de FILA (undefined = no tocar). */
  row?: string;
  /** Crudo a fijar en la celda de entrada de COLUMNA (undefined = no tocar). */
  col?: string;
}

/**
 * Recorre el plan: por cada celda lee el crudo de sus cabeceras (`read`) y evalúa
 * la fórmula tras fijar las entradas (`evaluate`, que el lienzo implementa con un
 * motor desechable). PURA por inyección — el test la ejercita con lecturas y un
 * evaluador propios, y también respaldada por el motor real (como el informe de
 * escenarios).
 */
export const computeDataTable = (
  plan: DataTablePlan,
  read: (c: number, r: number) => string,
  evaluate: (subs: DataTableSubs) => CellValue,
): DataTableResultCell[] =>
  plan.cells.map((cell) => {
    const subs: DataTableSubs = {};
    if (cell.rowFrom) subs.row = read(cell.rowFrom.c, cell.rowFrom.r);
    if (cell.colFrom) subs.col = read(cell.colFrom.c, cell.colFrom.r);
    return { c: cell.c, r: cell.r, value: evaluate(subs) };
  });
