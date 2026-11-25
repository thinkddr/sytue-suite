/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `@sytue/formula/motor`: el motor SIN las funciones de la librería (~245 KB), que se
 * piden aparte con `cargarFunciones()` (ver perezosas.ts). Es lo que importa el editor de
 * hojas: así las funciones no bajan con él, sino en su propio trozo y a la vez. Lleva
 * también lo de `./ligero` y los nombres de las funciones para el autocompletado.
 */
export * from './ligero.js';
export { Engine, VOLATILES, isValidName as isValidDefinedName } from './engine.js';
export type { EngineOptions } from './engine.js';
export { cargarFunciones, funcionesCargadas } from './perezosas.js';
export {
  FUNCIONES_DEL_MOTOR,
  nombresDeFuncionesSinRegistro as nombresDeFunciones,
} from './nombres-funciones.js';
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
