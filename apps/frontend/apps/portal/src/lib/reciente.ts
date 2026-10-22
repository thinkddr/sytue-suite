/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 */
import type { ThemeForma } from '@sytue/ui';

/** Cuántas entradas enseña «Reciente» en la forma «Marco»: el resto está en «Ver todo». */
export const RECIENTES_EN_MARCO = 3;

/**
 * Las entradas de «Reciente» que se PINTAN en cada forma.
 *
 * En «Marco» las que sobran no se renderizan. Antes se pintaban todas y se tapaban a
 * partir de la cuarta (`marco:hidden`), y la lista —un `divide-y`, que pone el filete a
 * todo lo que no sea el último hijo— seguía contando las tapadas: la tercera, que es
 * la última a la vista, llevaba un filete debajo sin nada que separar.
 */
export function recientesVisibles<T>(recientes: readonly T[], forma: ThemeForma): readonly T[] {
  return forma === 'marco' ? recientes.slice(0, RECIENTES_EN_MARCO) : recientes;
}
