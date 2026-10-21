/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Navegación por teclado de la rejilla (patrón «grid» de WAI-ARIA): flechas, Inicio/Fin,
 * Ctrl+Inicio/Ctrl+Fin y RePág/AvPág. Pura, para probarla sin navegador.
 */
export interface Celda {
  fila: number;
  col: number;
}

const SALTO = 10;

/** Dónde cae el foco tras pulsar `tecla`, o null si esa tecla no mueve. */
export function moverFoco(
  pos: Celda,
  tecla: string,
  ctrl: boolean,
  filas: number,
  cols: number,
): Celda | null {
  if (filas <= 0 || cols <= 0) return null;
  const f = Math.min(Math.max(pos.fila, 0), filas - 1);
  const c = Math.min(Math.max(pos.col, 0), cols - 1);
  switch (tecla) {
    case 'ArrowUp':
      return { fila: Math.max(f - 1, 0), col: c };
    case 'ArrowDown':
      return { fila: Math.min(f + 1, filas - 1), col: c };
    case 'ArrowLeft':
      return { fila: f, col: Math.max(c - 1, 0) };
    case 'ArrowRight':
      return { fila: f, col: Math.min(c + 1, cols - 1) };
    case 'Home':
      return ctrl ? { fila: 0, col: 0 } : { fila: f, col: 0 };
    case 'End':
      return ctrl ? { fila: filas - 1, col: cols - 1 } : { fila: f, col: cols - 1 };
    case 'PageUp':
      return { fila: Math.max(f - SALTO, 0), col: c };
    case 'PageDown':
      return { fila: Math.min(f + SALTO, filas - 1), col: c };
    default:
      return null;
  }
}
