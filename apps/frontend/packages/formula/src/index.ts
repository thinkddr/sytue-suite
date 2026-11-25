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
import { buildRegistry } from './functions/index.js';
import { instalarFunciones } from './perezosas.js';

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

// El índice trae las funciones e INSTALA su registro al cargarse: un `new Engine()` las
// tiene todas, como siempre. El editor de hojas importa `./motor`, que no las trae, y las
// pide aparte (perezosas.ts).
instalarFunciones(buildRegistry);
export { cargarFunciones, funcionesCargadas } from './perezosas.js';

export { CATALOGO_VERSION, FUNCIONES_DEL_MOTOR, catalogo, nombresDeFunciones } from './catalogo.js';
export type { Compatibilidad, Ejemplo, FuncionCatalogo } from './catalogo.js';
