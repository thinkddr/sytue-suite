/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La geometría de las formas poligonales y del cilindro vive ahora en
 * `@sytue/escena` (la escena vectorial común); este fichero la reexporta para
 * que Diapos y Docus conserven sus importaciones.
 */
export {
  cylinderBodyPath,
  cylinderDims,
  cylinderSeamPath,
  polygonAttr,
  polygonPoints,
} from '@sytue/escena';
export type { CylinderDims, ShapePoint } from '@sytue/escena';
