/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * @sytue/pages-render — el IR de Sytue Web (tipos, tabla de campos y validador) y, en sus
 * subrutas, el renderizador, el exportador y las islas. Corre en el navegador (lienzo, galería) y
 * en el BFF (publicar, exportar, vista previa).
 */

export * from './ir/tipos';
export { CAMPOS_NODO, CAMPOS_OBJETO, ENUMERADOS, UNIONES } from './ir/campos';
export type { CamposExactos } from './ir/campos';
export { validar, bytesIR, urlValida } from './ir/validar';
export type { CatalogoValidacion, CodigoProblema, ModoValidacion, Problema } from './ir/validar';
export { nuevoId } from './ir/ids';
export type { DatosLegales, DatosRender, DatosTienda, ProductoRender } from './contexto';
export { contraste } from './contraste';
export {
  RENDERER_VERSION,
  aumentar,
  estimarSalida,
  exportarAstro,
  publicar,
} from './exportar/index';
export type { Salida } from './exportar/index';
export { ORIGENES_DE_INCRUSTADOS, pintarPagina } from './pintar/index';
