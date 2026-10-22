/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * @sytue/escena — la escena vectorial común de la suite (Docus, Diapos, Pages).
 * Modelo (`tipos`), operaciones puras (`escena`), serialización versionada
 * (`serializar`), hit-testing (`golpe`), exportación a SVG (`svg`) y la geometría
 * compartida: formas, conectores y organización (z-order, alinear, distribuir).
 */
export * from './tipos.js';
export * from './escena.js';
export * from './serializar.js';
export * from './golpe.js';
export * from './svg.js';
export * from './formas.js';
export * from './conector.js';
export * from './organizar.js';
export * from './trazo.js';
export * from './diagramas.js';
export { aPdf, anchoTexto, caminoPdf } from './pdf.js';
export {
  IDENTIDAD,
  aplicar,
  componer,
  girar,
  girarEn,
  invertir,
  matrizSvg,
  trasladar,
} from './matriz.js';
export type { Matriz } from './matriz.js';
