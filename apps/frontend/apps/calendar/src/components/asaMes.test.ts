/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * VECTORES ESPEJO del asa del mes del teléfono. Los mismos, con el mismo nombre, están en
 * Fyne (`apps/fyne/calendario/asames_test.go`): si uno cambia aquí, cambia allí.
 *
 * Con 5 semanas la hoja mide 224 en el mes, 64 en la semana y 0 en la agenda; con 6, 264.
 *
 * Vistos en ROJO: sin el umbral de toque (A1c: un temblor rápido saltaba a la semana), con
 * el lanzamiento contado desde la posición de partida y no desde lo alcanzado (A5f: se
 * quedaba en la semana), midiendo la distancia desde el alto de partida (A3, A5, A7, A13),
 * sin recortar el arrastre (B2), con la velocidad desde la primera muestra del gesto (C1)
 * y con el desempate hacia fuera de la posición de partida (A14).
 */
import { describe, expect, test } from 'bun:test';
import {
  alturaArrastre,
  alturaHoja,
  geometriaHoja,
  posicionAlSoltar,
  posicionAlTocar,
  posicionDeVista,
  posicionTrasPaso,
  velocidadFinal,
  vistaDePosicion,
  type PosicionAsa,
} from './asaMes.js';

describe('posicionAlSoltar', () => {
  const casos: [string, PosicionAsa, number, number, number, PosicionAsa][] = [
    // id, desde, dy, velocidad, filas, se espera
    ['A1 sin-moverse', 'mes', -3, 0, 5, 'mes'],
    ['A1b poco-pero-arrastre', 'mes', -7, 0, 5, 'mes'],
    ['A1c temblor-rapido', 'mes', -5, -1, 5, 'mes'],
    ['A2 sube-poco-lento', 'mes', -60, 0, 5, 'mes'],
    ['A3 sube-pasada-la-mitad', 'mes', -100, 0, 5, 'semana'],
    ['A4 lanzamiento-corto-arriba', 'mes', -20, -0.8, 5, 'semana'],
    ['A5 sube-casi-todo-lento', 'mes', -200, 0, 5, 'agenda'],
    ['A5f lanzamiento-pasada-la-semana', 'mes', -170, -0.6, 5, 'agenda'],
    ['A6 semana-lanzada-arriba', 'semana', -10, -0.6, 5, 'agenda'],
    ['A7 agenda-baja-lento', 'agenda', 40, 0, 5, 'semana'],
    ['A8 agenda-lanzada-abajo', 'agenda', 15, 0.9, 5, 'semana'],
    ['A9 semana-lanzada-abajo', 'semana', 12, 0.7, 5, 'mes'],
    ['A10 mes-lanzado-abajo', 'mes', 20, 1, 5, 'mes'],
    ['A11 agenda-lanzada-arriba', 'agenda', -20, -1, 5, 'agenda'],
    ['A12 lanzamiento-contrario', 'mes', -120, 0.7, 5, 'mes'],
    ['A13 seis-semanas', 'mes', -130, 0, 6, 'semana'],
    ['A14 empate-se-queda', 'mes', -80, 0, 5, 'mes'],
    ['A15 lento-por-debajo-del-lanzamiento', 'semana', -20, -0.49, 5, 'semana'],
  ];
  for (const [id, desde, dy, v, filas, quiero] of casos) {
    test(id, () => expect(posicionAlSoltar(desde, dy, v, filas)).toBe(quiero));
  }
});

describe('alturas', () => {
  test('B1 alturas-de-cada-posicion', () => {
    expect([alturaHoja('mes', 5), alturaHoja('semana', 5), alturaHoja('agenda', 5)]).toEqual([
      224, 64, 0,
    ]);
    expect(alturaHoja('mes', 6)).toBe(264);
  });
  test('B2 el-arrastre-no-se-sale', () => {
    expect(alturaArrastre('mes', 50, 5)).toBe(224);
    expect(alturaArrastre('agenda', -30, 5)).toBe(0);
    expect(alturaArrastre('semana', -10, 5)).toBe(54);
  });
  test('B3 geometria-continua', () => {
    expect(geometriaHoja(224, 5, 2)).toEqual({ plegado: 0, subida: 0 });
    expect(geometriaHoja(144, 5, 2)).toEqual({ plegado: 40, subida: 0 });
    expect(geometriaHoja(64, 5, 2)).toEqual({ plegado: 80, subida: 0 });
    expect(geometriaHoja(32, 5, 2)).toEqual({ plegado: 80, subida: 32 });
    expect(geometriaHoja(0, 5, 0)).toEqual({ plegado: 0, subida: 64 });
  });
});

describe('velocidadFinal', () => {
  test('C1 ultimos-100-ms', () => {
    const m = [
      { t: 0, y: 0 },
      { t: 40, y: -10 },
      { t: 100, y: -40 },
      { t: 140, y: -80 },
    ];
    expect(velocidadFinal(m)).toBeCloseTo(-0.7, 6);
  });
  test('C2 una-muestra-o-ninguna', () => {
    expect(velocidadFinal([])).toBe(0);
    expect(velocidadFinal([{ t: 5, y: 9 }])).toBe(0);
    expect(
      velocidadFinal([
        { t: 5, y: 9 },
        { t: 5, y: 30 },
      ]),
    ).toBe(0);
  });
});

describe('toque, teclado y vistas', () => {
  test('D1 toque', () => {
    expect(['mes', 'semana', 'agenda'].map((p) => posicionAlTocar(p as PosicionAsa))).toEqual([
      'semana',
      'mes',
      'semana',
    ]);
  });
  test('D2 paso', () => {
    expect(posicionTrasPaso('mes', 1)).toBe('semana');
    expect(posicionTrasPaso('semana', 1)).toBe('agenda');
    expect(posicionTrasPaso('agenda', 1)).toBe('agenda');
    expect(posicionTrasPaso('mes', -1)).toBe('mes');
    expect(posicionTrasPaso('agenda', -2)).toBe('mes');
  });
  test('D3 vistas', () => {
    expect(posicionDeVista('day')).toBeNull();
    for (const p of ['mes', 'semana', 'agenda'] as PosicionAsa[]) {
      expect(posicionDeVista(vistaDePosicion(p))).toBe(p);
    }
  });
});
