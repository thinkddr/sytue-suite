/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Deshacer y rehacer inmutable con tope configurable.
 *
 * Extraído desde packages/office/src/editor/sheets/historial.ts para ser compartido
 * por las herramientas de edición de Sytue One (Docs, Sheets, Slides, Pages).
 */

/** Pila de deshacer y de rehacer. Inmutable: cada operación devuelve una nueva. */
export interface Historial<T> {
  pasado: readonly T[];
  futuro: readonly T[];
}

/** Cuántos pasos atrás se guardan por defecto (50 para Sheets). */
export const TOPE = 50;

export function nuevoHistorial<T>(): Historial<T> {
  return { pasado: [], futuro: [] };
}

/**
 * Anota el estado ANTERIOR a un cambio.
 *
 * Vacía el futuro: si tras deshacer se escribe algo nuevo, aquellos pasos ya no
 * se pueden rehacer (la historia se bifurcó).
 */
export function anotar<T>(h: Historial<T>, anterior: T, tope = TOPE): Historial<T> {
  const pasado = [...h.pasado, anterior];
  return { pasado: pasado.length > tope ? pasado.slice(pasado.length - tope) : pasado, futuro: [] };
}

/** Un paso atrás. null si no hay nada que deshacer. */
export function deshacer<T>(
  h: Historial<T>,
  actual: T,
): { historial: Historial<T>; modelo: T } | null {
  if (h.pasado.length === 0) return null;
  const modelo = h.pasado[h.pasado.length - 1] as T;
  return {
    historial: { pasado: h.pasado.slice(0, -1), futuro: [...h.futuro, actual] },
    modelo,
  };
}

/** Un paso adelante. null si no hay nada que rehacer. */
export function rehacer<T>(
  h: Historial<T>,
  actual: T,
): { historial: Historial<T>; modelo: T } | null {
  if (h.futuro.length === 0) return null;
  const modelo = h.futuro[h.futuro.length - 1] as T;
  return {
    historial: { pasado: [...h.pasado, actual], futuro: h.futuro.slice(0, -1) },
    modelo,
  };
}

export const sePuedeDeshacer = (h: Historial<unknown>): boolean => h.pasado.length > 0;
export const sePuedeRehacer = (h: Historial<unknown>): boolean => h.futuro.length > 0;
