/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Núcleo puro de arrastre, redimensión y navegación por teclado en lienzos de edición.
 *
 * Extraído de SlidesCanvas.tsx:3710-3799 para compartir entre Slides y Pages.
 */

export interface Coordenadas {
  x: number;
  y: number;
}

export interface Dimensiones {
  w: number;
  h: number;
}

export interface Rectangulo extends Coordenadas, Dimensiones {}

export interface RectanguloConId extends Rectangulo {
  id: string;
}

export interface Limites {
  x: number;
  y: number;
}

export function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max);
}

/**
 * Calcula la caja envolvente (bounding box) de un conjunto de rectángulos.
 */
export function calcularEnvolvente(cajas: readonly Rectangulo[]): Rectangulo {
  if (cajas.length === 0) return { x: 0, y: 0, w: 0, h: 0 };
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const c of cajas) {
    if (c.x < minX) minX = c.x;
    if (c.y < minY) minY = c.y;
    if (c.x + c.w > maxX) maxX = c.x + c.w;
    if (c.y + c.h > maxY) maxY = c.y + c.h;
  }
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
}

/**
 * Calcula los límites de desplazamiento permitidos para que la envolvente no
 * se salga de las dimensiones del lienzo (por defecto 100x100 en porcentaje).
 */
export function calcularLimitesArrastre(
  envolvente: Rectangulo,
  limiteLienzo: Dimensiones = { w: 100, h: 100 },
): { min: Limites; max: Limites } {
  return {
    min: { x: -envolvente.x, y: -envolvente.y },
    max: {
      x: limiteLienzo.w - (envolvente.x + envolvente.w),
      y: limiteLienzo.h - (envolvente.y + envolvente.h),
    },
  };
}

/**
 * Calcula el arrastre de una o varias cajas manteniendo sus posiciones relativas.
 * Aplica clamp al delta según los límites mínimo y máximo.
 */
export function calcularArrastre<T extends RectanguloConId>(
  origs: readonly T[],
  dxRaw: number,
  dyRaw: number,
  min: Limites,
  max: Limites,
): { dx: number; dy: number; nuevasPosiciones: T[] } {
  const dx = clamp(dxRaw, min.x, max.x);
  const dy = clamp(dyRaw, min.y, max.y);
  const nuevasPosiciones = origs.map((o) => ({
    ...o,
    x: o.x + dx,
    y: o.y + dy,
  }));
  return { dx, dy, nuevasPosiciones };
}

export type TiradorRedimension = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw';

/**
 * Calcula la redimensión de una caja a partir de un delta, aplicando topes mínimo y máximo.
 * Por defecto redimensiona desde la esquina inferior derecha ('se'), coincidiendo
 * con el comportamiento estándar de Diapos.
 */
export function calcularRedimension(
  orig: Rectangulo,
  dxRaw: number,
  dyRaw: number,
  minLado = 4,
  maxLado = 100,
  tirador: TiradorRedimension = 'se',
): Rectangulo {
  let { x, y, w, h } = orig;

  if (tirador.includes('e')) {
    w = clamp(orig.w + dxRaw, minLado, maxLado - orig.x);
  } else if (tirador.includes('w')) {
    const nuevoW = clamp(orig.w - dxRaw, minLado, orig.x + orig.w);
    x = orig.x + (orig.w - nuevoW);
    w = nuevoW;
  }

  if (tirador.includes('s')) {
    h = clamp(orig.h + dyRaw, minLado, maxLado - orig.y);
  } else if (tirador.includes('n')) {
    const nuevoH = clamp(orig.h - dyRaw, minLado, orig.y + orig.h);
    y = orig.y + (orig.h - nuevoH);
    h = nuevoH;
  }

  return { x, y, w, h };
}

/**
 * Calcula el paso de teclado para movimiento o redimensión según la tecla de flecha.
 */
export function calcularPasoTeclado(
  key: string,
  multiplicador = 1,
): { dx: number; dy: number } | null {
  if (!key.startsWith('Arrow')) return null;
  const dx = (key === 'ArrowLeft' ? -1 : key === 'ArrowRight' ? 1 : 0) * multiplicador;
  const dy = (key === 'ArrowUp' ? -1 : key === 'ArrowDown' ? 1 : 0) * multiplicador;
  return { dx, dy };
}
