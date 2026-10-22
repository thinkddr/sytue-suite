/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { fichasDe, mereceInterpretarse } from './interpretacion.js';

// El filtro es de coste: una llamada de pago por cada tecla de «fact» mientras
// alguien escribe «factura» sería pagar seis veces por nada.
test('una o dos palabras no dan para interpretar', () => {
  expect(mereceInterpretarse('factura')).toBe(false);
  expect(mereceInterpretarse('  factura   acme ')).toBe(false);
  expect(mereceInterpretarse('facturas de Acme del mes pasado')).toBe(true);
  expect(mereceInterpretarse('x '.repeat(400))).toBe(false);
});

const t = {
  fuente: (n: string) => `en ${n}`,
  tipo: (k: string) => `solo ${k}`,
  rango: (d: string, h: string) => `${d} – ${h}`,
  desde: (d: string) => `desde ${d}`,
  hasta: (h: string) => `hasta ${h}`,
};
const nombre = (s: string) => s.toUpperCase();
const fecha = (ms: number) => new Date(ms).toISOString().slice(0, 10);

test('cada filtro aplicado sale como una ficha', () => {
  const f = fichasDe(
    {
      text: 'factura',
      sources: ['drive'],
      kinds: ['file'],
      from: Date.parse('2026-08-01T00:00:00Z'),
      to: Date.parse('2026-08-31T00:00:00Z'),
    },
    nombre,
    nombre,
    fecha,
    t,
  );
  expect(f.map((x) => x.texto)).toEqual(['en DRIVE', 'solo FILE', '2026-08-01 – 2026-08-31']);
});

// El término literal NO es una ficha: ya está en el campo, a la vista.
test('sin filtros no hay nada que enseñar', () => {
  expect(fichasDe({ text: 'memoria anual' }, nombre, nombre, fecha, t)).toEqual([]);
});

test('un solo extremo del rango se cuenta como tal', () => {
  const soloDesde = fichasDe(
    { text: '', from: Date.parse('2026-08-01T00:00:00Z') },
    nombre,
    nombre,
    fecha,
    t,
  );
  expect(soloDesde[0]?.texto).toBe('desde 2026-08-01');
  const soloHasta = fichasDe(
    { text: '', to: Date.parse('2026-08-01T00:00:00Z') },
    nombre,
    nombre,
    fecha,
    t,
  );
  expect(soloHasta[0]?.texto).toBe('hasta 2026-08-01');
});
