/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Adaptador de historial inmutable para Pages con tope 100 (§14.1).
 */

import {
  type Historial,
  anotar,
  deshacer,
  nuevoHistorial,
  rehacer,
  sePuedeDeshacer,
  sePuedeRehacer,
} from '@sytue/ui';
import type { Sitio } from '@sytue/pages-render';

/** Tope de 100 pasos configurado según especificación §14.1 */
export const TOPE_PAGES = 100;

export type HistorialPages = Historial<Sitio>;

export function crearHistorialEditor(): HistorialPages {
  return nuevoHistorial<Sitio>();
}

export function registrarCambio(h: HistorialPages, anterior: Sitio): HistorialPages {
  return anotar(h, anterior, TOPE_PAGES);
}

export function deshacerPages(
  h: HistorialPages,
  actual: Sitio,
): { historial: HistorialPages; modelo: Sitio } | null {
  return deshacer(h, actual);
}

export function rehacerPages(
  h: HistorialPages,
  actual: Sitio,
): { historial: HistorialPages; modelo: Sitio } | null {
  return rehacer(h, actual);
}

export function puedeDeshacerPages(h: HistorialPages): boolean {
  return sePuedeDeshacer(h);
}

export function puedeRehacerPages(h: HistorialPages): boolean {
  return sePuedeRehacer(h);
}

export {
  deshacer,
  rehacer,
  sePuedeDeshacer,
  sePuedeRehacer,
  crearHistorialEditor as crearHistorialPages,
  registrarCambio as registrarCambioPages,
};
export type { Historial };
