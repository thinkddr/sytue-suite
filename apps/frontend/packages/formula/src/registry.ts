/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CONTRATO de una función de librería y registro por nombre (insensible a
 * mayúsculas). Este es el contrato que siguen los ficheros de src/functions/*.
 */
import type { CellValue, FormulaError } from './values.js';
import type { TableArea } from './parser.js';

/** Rango resuelto: rectángulo de celdas [c1..c2] × [r1..r2] en una hoja. 0-based. */
export interface RefRange {
  sheet: string;
  c1: number;
  r1: number;
  c2: number;
  r2: number;
}

/**
 * Petición de resolución de una referencia estructurada `Tabla[Columna]`.
 * - `table`   nombre de la tabla; '' si es implícita (la que contiene la celda).
 * - `column`  columna pedida; undefined si solo se pide un área (`Tabla[#Todo]`).
 * - `area`    área de la tabla (datos por defecto).
 * - `thisRow` `@` → intersecar con la fila de la celda que evalúa.
 */
export interface TableQuery {
  table: string;
  column: string | undefined;
  area: TableArea;
  thisRow: boolean;
}

/**
 * Resolutor INYECTABLE de tablas (lo provee el editor, dueño del mapa
 * tabla→rango). Mapea una petición estructurada + la celda que evalúa a un
 * RefRange. Devolver:
 *  - un RefRange → se lee como un rango normal;
 *  - un FormulaError → ese error concreto (p. ej. #VALUE! si `@` cae fuera de la
 *    tabla, #REF! si la columna no existe, #NAME? si la tabla no existe);
 *  - undefined → el motor asume #REF! (tabla/columna desconocida o sin resolutor).
 */
export type TableResolver = (
  q: TableQuery,
  here: { sheet: string; col: number; row: number },
) => RefRange | FormulaError | undefined;

/**
 * Un argumento de función, PEREZOSO. El motor no evalúa el argumento hasta que
 * la función lo pide. Esto es lo que permite el cortocircuito de IF/AND/OR/…:
 * una función evalúa `value()`/`matrix()` solo de los argumentos que necesita.
 *
 * - `value()`  → el argumento como escalar. Colapsa un rango 1×1 a su valor;
 *                un rango multi-celda en contexto escalar da #VALUE!. Los
 *                resultados se memorizan (llamar dos veces no re-evalúa).
 * - `matrix()` → el argumento como matriz `CellValue[][]` (filas × columnas).
 *                Un escalar se envuelve en `[[valor]]`. Úsalo en SUM, COUNT…
 * - `ref`      → si el argumento es una referencia/rango LITERAL (A1, A1:B2,
 *                Hoja1!A1), su RefRange resuelto; si no (número, llamada…),
 *                undefined. Úsalo en ROW/COLUMN/OFFSET/CELL, que necesitan la
 *                referencia y no su valor.
 */
export interface Arg {
  value: () => CellValue;
  matrix: () => CellValue[][];
  readonly ref: RefRange | undefined;
  /**
   * ¿El argumento es un VALOR escrito en la fórmula —un texto entre comillas o el resultado
   * de un operador («2», "1"&"0")— y no una referencia ni una llamada? Es lo que Excel
   * convierte a número en SUM, AVERAGE, COUNT… Lo que devuelve una función (INDEX, OFFSET)
   * no lo es: en Excel puede ser una referencia, cuyo texto se ignora.
   */
  readonly literal?: boolean;
}

/**
 * Contexto de ejecución que el motor pasa a cada función.
 * - `now()`      reloj inyectable (NOW/TODAY). Determinista en tests.
 * - `random()`   fuente de aleatoriedad inyectable (RAND/RANDBETWEEN).
 * - `here`       celda que se está evaluando (0-based) para ROW()/COLUMN()/…
 * - `resolve()`  lee un RefRange como matriz de valores (INDEX/OFFSET…).
 * - `parseRef()` parsea texto «Hoja1!A1:B2» a RefRange (INDIRECT). Hoja por
 *                defecto = la de la celda actual. undefined si no es una ref.
 * - `isFormula()` ¿la celda superior-izquierda del rango es una fórmula «=»?
 *                (ISFORMULA). Inspecciona la forma, no el valor.
 * - `resolveTable()` resuelve una referencia estructurada `Tabla[Columna]` a un
 *                RefRange (o error) vía el resolutor inyectado por el editor;
 *                queda anclado a la celda actual (para `@`/tabla implícita).
 */
export interface FnCtx {
  now: () => Date;
  random: () => number;
  readonly here: { sheet: string; col: number; row: number };
  resolve: (ref: RefRange) => CellValue[][];
  parseRef: (text: string) => RefRange | undefined;
  isFormula: (ref: RefRange) => boolean;
  resolveTable: (q: TableQuery) => RefRange | FormulaError | undefined;
  /**
   * El texto de la fórmula de la celda superior-izquierda del rango, tal como se escribió
   * («=A1+1»), o undefined si no es una fórmula (FORMULATEXT). Opcional por lo mismo que
   * `subtotalCell`.
   */
  formulaText?: (ref: RefRange) => string | undefined;
  /** Nombres de las hojas del libro, en orden (SHEET, SHEETS). Opcional. */
  sheetNames?: () => string[];
  /**
   * ¿La celda es un SUBTOTAL o un AGGREGATE (o los lleva dentro)? Es lo que esas dos
   * funciones se saltan para no contar dos veces lo que ya resume otra. Opcional para
   * que los contextos de prueba que se escriben a mano sigan valiendo: sin él, nada
   * cuenta como anidado.
   */
  subtotalCell?: (sheet: string, col: number, row: number) => boolean;
  /**
   * Cómo está la fila: 0 se ve, 1 oculta a mano (un grupo del esquema contraído) y
   * 2 oculta por el filtro. SUBTOTAL 1-11 salta solo las filtradas; 101-111 y las
   * opciones «ocultas» de AGGREGATE, las dos. Sin él, todas se ven.
   */
  rowHidden?: (sheet: string, row: number) => 0 | 1 | 2;
}

/** Lo que devuelve una función: un escalar o una matriz (rango calculado). */
export type FnResult = CellValue | CellValue[][];

/**
 * La firma de TODA función de librería. Recibe el ctx y los argumentos
 * perezosos; devuelve un escalar o una matriz. Para propagar errores, evalúa
 * el argumento (con `value()`/`matrix()`) y, si es un FormulaError, devuélvelo.
 */
export type FnImpl = (ctx: FnCtx, args: Arg[]) => FnResult;

/** Tabla nombre→implementación. Las claves pueden ir en cualquier caja. */
export type FnTable = Record<string, FnImpl>;

/** Registro de funciones. Búsqueda insensible a mayúsculas. */
export class Registry {
  private readonly m = new Map<string, FnImpl>();

  add(table: FnTable): this {
    for (const name of Object.keys(table)) this.m.set(name.toUpperCase(), table[name]!);
    return this;
  }
  get(name: string): FnImpl | undefined {
    return this.m.get(name.toUpperCase());
  }
  has(name: string): boolean {
    return this.m.has(name.toUpperCase());
  }
  /** Nombres (en MAYÚSCULAS) de todas las funciones registradas, ordenados.
   * Para el autocompletado de fórmulas del editor: cobertura completa y en vivo
   * (incluye funciones añadidas por cualquier tabla) sin exponer las implementaciones. */
  names(): string[] {
    return [...this.m.keys()].sort();
  }
}
