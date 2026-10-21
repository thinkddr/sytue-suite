/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Transformaciones afines 2D, puras. Una matriz es [a, b, c, d, e, f] con la
 * convención de SVG (`matrix(a b c d e f)`): x' = a·x + c·y + e, y' = b·x + d·y + f.
 * La escena solo usa movimientos rígidos (traslación + giro), pero la inversa y la
 * composición son las generales.
 */
import type { Punto } from './tipos.js';

export type Matriz = readonly [number, number, number, number, number, number];

export const IDENTIDAD: Matriz = [1, 0, 0, 1, 0, 0];

const RAD = Math.PI / 180;

/** `m · n`: aplica primero `n` y después `m`. */
export const componer = (m: Matriz, n: Matriz): Matriz => [
  m[0] * n[0] + m[2] * n[1],
  m[1] * n[0] + m[3] * n[1],
  m[0] * n[2] + m[2] * n[3],
  m[1] * n[2] + m[3] * n[3],
  m[0] * n[4] + m[2] * n[5] + m[4],
  m[1] * n[4] + m[3] * n[5] + m[5],
];

export const trasladar = (dx: number, dy: number): Matriz => [1, 0, 0, 1, dx, dy];

/** Giro en grados (sentido horario en pantalla, como `rotate()` de SVG) alrededor del origen. */
export const girar = (grados: number): Matriz => {
  const c = Math.cos(grados * RAD);
  const s = Math.sin(grados * RAD);
  return [c, s, -s, c, 0, 0];
};

/** Giro de `grados` alrededor de (cx, cy). */
export const girarEn = (grados: number, cx: number, cy: number): Matriz =>
  componer(trasladar(cx, cy), componer(girar(grados), trasladar(-cx, -cy)));

export const aplicar = (m: Matriz, p: Punto): Punto => ({
  x: m[0] * p.x + m[2] * p.y + m[4],
  y: m[1] * p.x + m[3] * p.y + m[5],
});

/** Inversa, o `undefined` si la matriz es singular. */
export function invertir(m: Matriz): Matriz | undefined {
  const det = m[0] * m[3] - m[1] * m[2];
  if (!Number.isFinite(det) || Math.abs(det) < 1e-12) return undefined;
  const a = m[3] / det;
  const b = -m[1] / det;
  const c = -m[2] / det;
  const d = m[0] / det;
  return [a, b, c, d, -(a * m[4] + c * m[5]), -(b * m[4] + d * m[5])];
}

/** Cadena `matrix(...)` de SVG, con 4 decimales como mucho. */
export const matrizSvg = (m: Matriz): string =>
  `matrix(${m.map((v) => redondear(v, 4)).join(' ')})`;

function redondear(v: number, dec: number): number {
  const k = 10 ** dec;
  const r = Math.round(v * k) / k;
  return Object.is(r, -0) ? 0 : r;
}
