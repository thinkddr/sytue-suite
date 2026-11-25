/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `@sytue/formula/ligero`: lo del paquete que NO es el motor. El parser de fórmulas y de
 * referencias, los valores con sus errores y el formato de número, sin `Engine` ni el
 * registro de funciones (~245 KB).
 *
 * Lo usan las piezas del editor que también se ven en un documento o una presentación
 * (los gráficos, los comentarios anclados a una celda, el formato de número): con el
 * índice del paquete arrastraban el motor entero, y las fórmulas solo hacen falta en
 * una hoja. Son las mismas funciones que exporta el índice, no una copia.
 */
export {
  parseFormula,
  ParseError,
  colToNum,
  numToCol,
  parseCellWord,
  parseRefEnd,
  parseBracketSpec,
} from './parser.js';
export type { AstNode, CellRef, BinOp, TableArea, RefEnd, RefEndKind } from './parser.js';

export {
  FormulaError,
  ERR,
  isError,
  coerceNumber,
  coerceText,
  coerceBoolean,
  compareValues,
  formatNumber,
} from './values.js';
export type { CellValue, ErrorCode } from './values.js';
