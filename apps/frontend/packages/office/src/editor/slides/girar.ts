/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Giro de las cajas de una diapositiva (`SlideBox.rot`, grados en sentido horario
 * alrededor del centro, como el `rot` de PowerPoint y el `rotate` de CSS). Aquí solo
 * va lo propio de Diapos —las cajas en % de un lienzo que no es cuadrado—; la matriz
 * del giro es la de `@sytue/escena`. Pruebas: test/slides-girar.test.ts.
 */
import { aplicar, girar } from '@sytue/escena';
import type { SlideBox } from '../types';

/** Paso del giro con Mayús (y del asa al acercarse a un múltiplo): el de PowerPoint. */
export const PASO_GIRO = 15;

/** Grados en [0, 360) con tres decimales, o `undefined` si no hay giro (0, o algo que no es
 *  un número): una caja sin girar no lleva el campo, así un documento previo se queda igual. */
export function normalizarGiro(grados: unknown): number | undefined {
  if (typeof grados !== 'number' || !Number.isFinite(grados)) return undefined;
  const r = Math.round((((grados % 360) + 360) % 360) * 1000) / 1000;
  return r === 0 || r === 360 ? undefined : r;
}

/** El giro de una caja tal y como se pinta: 0 si no lleva o lleva algo inválido. */
export const giroDe = (b: Pick<SlideBox, 'rot'>): number => normalizarGiro(b.rot) ?? 0;

/** Estilo CSS del giro. Va en la propiedad `rotate` y no en `transform` porque las
 *  animaciones de la presentación (volar, zoom, énfasis) animan `transform`: con el giro
 *  ahí, una caja girada se enderezaría mientras se anima. */
export function estiloGiro(b: Pick<SlideBox, 'rot'>): { rotate?: string } {
  const g = giroDe(b);
  return g === 0 ? {} : { rotate: `${g}deg` };
}

/** Caja con su giro puesto (o quitado si queda en 0). Pura. */
export function conGiro(b: SlideBox, grados: number): SlideBox {
  const { rot: _antes, ...resto } = b;
  const g = normalizarGiro(grados);
  return g === undefined ? resto : { ...resto, rot: g };
}

/** Suma `delta` grados al giro de las cajas `ids` (`null` = las endereza). Cada una gira
 *  sobre su centro, como en PowerPoint con varias seleccionadas. Pura. */
export function girarCajas(
  boxes: SlideBox[],
  ids: readonly string[],
  delta: number | null,
): SlideBox[] {
  const sel = new Set(ids);
  return boxes.map((b) => (sel.has(b.id) ? conGiro(b, delta === null ? 0 : giroDe(b) + delta) : b));
}

/**
 * Ángulo del asa de giro: el que forma el puntero con el centro de la caja, medido desde
 * «arriba» (donde está el asa) en sentido horario y en px de pantalla. Con `paso` se
 * redondea a sus múltiplos (Mayús: de 15 en 15°).
 */
export function anguloDelPuntero(
  centro: { x: number; y: number },
  puntero: { x: number; y: number },
  paso = 0,
): number {
  const rad = Math.atan2(puntero.x - centro.x, centro.y - puntero.y);
  let g = (rad * 180) / Math.PI;
  if (paso > 0) g = Math.round(g / paso) * paso;
  return normalizarGiro(g) ?? 0;
}

/**
 * Redimensiona una caja girada desde su esquina inferior derecha. El arrastre llega en
 * px de pantalla; se lleva a los ejes de la caja (deshaciendo el giro) y, como el giro
 * es alrededor del centro, se recoloca la caja para que la esquina opuesta —la superior
 * izquierda, girada— no se mueva: es lo que hace PowerPoint. Sin giro coincide con el
 * redimensionado de siempre. `ancho`/`alto` son los px del lienzo (100 %); `min`, el
 * lado mínimo en %.
 */
export function redimensionarGirada(
  o: { x: number; y: number; w: number; h: number; rot?: number },
  dxPx: number,
  dyPx: number,
  ancho: number,
  alto: number,
  min = 4,
): { x: number; y: number; w: number; h: number } {
  const g = giroDe(o);
  const local = aplicar(girar(-g), { x: dxPx, y: dyPx });
  const wPx = Math.max((min / 100) * ancho, (o.w / 100) * ancho + local.x);
  const hPx = Math.max((min / 100) * alto, (o.h / 100) * alto + local.y);
  // Esquina superior izquierda girada, en px: es la que se queda quieta.
  const w0 = (o.w / 100) * ancho;
  const h0 = (o.h / 100) * alto;
  const c0 = { x: (o.x / 100) * ancho + w0 / 2, y: (o.y / 100) * alto + h0 / 2 };
  const m = girar(g);
  const v0 = aplicar(m, { x: -w0 / 2, y: -h0 / 2 });
  const fija = { x: c0.x + v0.x, y: c0.y + v0.y };
  const v1 = aplicar(m, { x: wPx / 2, y: hPx / 2 });
  const c1 = { x: fija.x + v1.x, y: fija.y + v1.y };
  const r = (v: number): number => Math.round(v * 1000) / 1000;
  return {
    x: r(((c1.x - wPx / 2) / ancho) * 100),
    y: r(((c1.y - hPx / 2) / alto) * 100),
    w: r((wPx / ancho) * 100),
    h: r((hPx / alto) * 100),
  };
}
