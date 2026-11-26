/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cuándo una factura está vencida (vencimiento.ts) y cómo reparte la tarjeta de la
 * antigüedad el ancho de sus barras.
 */
import { expect, test } from 'bun:test';
import { diasVencida } from './vencimiento.js';
import { anchoDeTramo } from './Cartera.js';
import type { Registro } from './tipos.js';

const factura = (estado: string, vencimiento?: string): Registro =>
  ({
    id: 'f1',
    modelo: 'factura_venta',
    estado,
    data: vencimiento ? { vencimiento } : {},
  }) as unknown as Registro;

const hoy = new Date(2026, 10, 25, 9, 30);

test('una factura abierta cuenta los días desde su vencimiento; el día del vencimiento, no', () => {
  expect(diasVencida(factura('confirmado', '2026-11-20'), hoy)).toBe(5);
  expect(diasVencida(factura('confirmado', '2026-11-25'), hoy)).toBe(0);
  expect(diasVencida(factura('confirmado', '2026-12-01'), hoy)).toBe(0);
});

test('pagada, anulada, en borrador o sin vencimiento no está vencida', () => {
  for (const estado of ['pagado', 'anulado', 'borrador']) {
    expect(diasVencida(factura(estado, '2026-01-01'), hoy)).toBe(0);
  }
  expect(diasVencida(factura('confirmado'), hoy)).toBe(0);
  expect(diasVencida(factura('confirmado', '01/01/2026'), hoy)).toBe(0);
});

test('a medianoche cuenta el día de quien mira, no el de UTC', () => {
  expect(diasVencida(factura('pendiente', '2026-11-25'), new Date(2026, 10, 26, 0, 30))).toBe(1);
});

test('cada tramo ocupa su parte del total, y uno con algo nunca desaparece', () => {
  expect(anchoDeTramo(50, 100)).toBe(50);
  expect(anchoDeTramo(1, 10_000)).toBe(2);
  expect(anchoDeTramo(0, 100)).toBe(0);
  expect(anchoDeTramo(10, 0)).toBe(0);
});
