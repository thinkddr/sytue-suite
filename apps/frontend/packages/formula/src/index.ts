/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * @sytue/formula — motor de fórmulas clean-room para el KIND spreadsheet del
 * editor Sytue Docs. Semántica OpenFormula / ECMA-376. Cero dependencias.
 *
 * API pública: el motor `Engine`, `parseFormula` (+ tipos del AST), los valores
 * y errores canónicos, y el contrato de función (Registry/FnCtx/Arg/…).
 */
export { Engine, numToCol, VOLATILES, isValidName as isValidDefinedName } from './engine.js';
export type { EngineOptions } from './engine.js';

export {
  parseFormula,
  ParseError,
  colToNum,
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

export { Registry } from './registry.js';
export type {
  Arg,
  FnCtx,
  FnImpl,
  FnTable,
  FnResult,
  RefRange,
  TableQuery,
  TableResolver,
} from './registry.js';

export { buildRegistry } from './functions/index.js';

export { CATALOGO_VERSION, FUNCIONES_DEL_MOTOR, catalogo, nombresDeFunciones } from './catalogo.js';
export type { Compatibilidad, Ejemplo, FuncionCatalogo } from './catalogo.js';
