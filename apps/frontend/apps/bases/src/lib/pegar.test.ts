/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * PEGAR UN BLOQUE DE UNA HOJA: los mismos casos que `TestPegarUnBloqueDeUnaHoja` y
 * `TestSoltarFicherosLosAdjunta` de Fyne (`apps/fyne/apps/bases/ficheros_test.go`).
 */
import { expect, test } from 'bun:test';
import type { BasesField, BasesRecord } from '@sytue/api-clients/bases';
import { bloque, campoDeAdjuntos, esCSV, leerFecha, planDePegado, valorPegado } from './pegar.js';

const campo = (id: string, kind: BasesField['kind'], extra: Partial<BasesField> = {}) =>
  ({ id, table_id: 't1', name: id, kind, position: 0, ...extra }) as BasesField;

const registro = (id: string, data: BasesRecord['data']): BasesRecord =>
  ({ id, table_id: 't1', rev: 1, data, links: {} }) as unknown as BasesRecord;

const NOM = campo('nom', 'text');
const EDA = campo('eda', 'number');

test('un bloque de una hoja se parte en filas y columnas (CRLF y salto final incluidos)', () => {
  expect(bloque('a\tb\r\nc\td\r\n')).toEqual([
    ['a', 'b'],
    ['c', 'd'],
  ]);
  expect(bloque('solo')).toEqual([['solo']]);
  expect(bloque('')).toEqual([]);
  expect(bloque('\n')).toEqual([]);
});

test('pegar 3×2 desde A1 escribe las seis celdas que cambian', () => {
  const rs = [
    registro('r1', { nom: 'P1', eda: 1 }),
    registro('r2', { nom: 'P2', eda: 2 }),
    registro('r3', { nom: 'P3', eda: 3 }),
  ];
  const plan = planDePegado([NOM, EDA], rs, { fila: 0, col: 0 }, 'A\t10\nB\t20\nC\t30');
  expect(plan.celdas.map((c) => [c.registro.id, c.datos])).toEqual([
    ['r1', { nom: 'A', eda: 10 }],
    ['r2', { nom: 'B', eda: 20 }],
    ['r3', { nom: 'C', eda: 30 }],
  ]);
  expect(plan.nuevos).toEqual([]);
  expect(plan.saltados).toBe(0);
});

test('como en Fyne: lo que no cabe son registros nuevos y lo que no encaja se cuenta', () => {
  const rs = [
    registro('r001', { nom: 'Persona 1', eda: 21 }),
    registro('r002', { nom: 'Persona 2', eda: 22 }),
  ];
  const plan = planDePegado(
    [NOM, EDA],
    rs,
    { fila: 0, col: 0 },
    'Ana\t30\r\nBea\tcuarenta\r\nCarla\t52,5\r\n',
  );
  expect(plan.celdas.map((c) => [c.registro.id, c.datos])).toEqual([
    ['r001', { nom: 'Ana', eda: 30 }],
    ['r002', { nom: 'Bea' }],
  ]);
  expect(plan.nuevos).toEqual([{ nom: 'Carla', eda: 52.5 }]);
  expect(plan.saltados).toBe(1);
});

test('se empieza en la celda con el foco, lo que se sale por la derecha se descarta y lo igual no se reescribe', () => {
  const rs = [registro('r1', { nom: 'Ana', eda: 30 }), registro('r2', { nom: 'Bea', eda: 1 })];
  const plan = planDePegado([NOM, EDA], rs, { fila: 1, col: 1 }, '40\tsobra\n50');
  expect(plan.celdas).toEqual([{ registro: rs[1]!, datos: { eda: 40 } }]);
  expect(plan.nuevos).toEqual([{ eda: 50 }]);
  expect(plan.saltados).toBe(0);
  // Un valor igual al que hay no se manda.
  expect(planDePegado([NOM, EDA], rs, { fila: 0, col: 0 }, 'Ana\t30').celdas).toEqual([]);
  // Fuera de la rejilla o vacío: nada.
  expect(planDePegado([NOM], rs, { fila: 0, col: 3 }, 'x').celdas).toEqual([]);
  expect(planDePegado([NOM], rs, { fila: 0, col: 0 }, '')).toEqual({
    celdas: [],
    nuevos: [],
    saltados: 0,
  });
});

test('las fórmulas, los enlaces y los adjuntos no se pegan: se cuentan si traían algo', () => {
  const campos = [campo('f', 'formula'), campo('l', 'link'), campo('a', 'attachment'), NOM];
  const plan = planDePegado(campos, [], { fila: 0, col: 0 }, '1\tx\t\tTexto');
  expect(plan.nuevos).toEqual([{ nom: 'Texto' }]);
  expect(plan.saltados).toBe(2);
});

test('cada tipo lee lo que deja una hoja', () => {
  const casilla = campo('c', 'checkbox');
  expect(valorPegado(casilla, 'TRUE')).toBe(true);
  expect(valorPegado(casilla, '✓')).toBe(true);
  expect(valorPegado(casilla, 'FALSE')).toBeNull();
  expect(valorPegado(casilla, '0')).toBeNull();
  const varios = campo('m', 'multiselect', { choices: ['Rojo', 'Azul'] });
  expect(valorPegado(varios, 'Rojo, Azul')).toEqual(['Rojo', 'Azul']);
  expect(valorPegado(varios, 'Verde')).toBeUndefined();
  expect(valorPegado(varios, '')).toBeNull();
  const fecha = campo('d', 'date');
  expect(valorPegado(fecha, '2026-10-05')).toBe('2026-10-05');
  expect(valorPegado(fecha, '05/10/2026')).toBe('2026-10-05');
  expect(valorPegado(fecha, '31/02/2026')).toBeUndefined();
  expect(valorPegado(campo('s', 'select', { choices: ['Sí'] }), 'Sí')).toBe('Sí');
  expect(valorPegado(EDA, ' 7 ')).toBe(7);
  expect(valorPegado(NOM, '')).toBeNull();
});

test('las fechas como se escriben en Europa', () => {
  expect(leerFecha('5/10/2026')).toBe('2026-10-05');
  expect(leerFecha('05.10.2026')).toBe('2026-10-05');
  expect(leerFecha('05-10-2026')).toBe('2026-10-05');
  expect(leerFecha('05102026')).toBe('2026-10-05');
  expect(leerFecha('2026/10/5')).toBe('2026-10-05');
  expect(leerFecha('29/02/2025')).toBeUndefined();
  expect(leerFecha('5/10/26')).toBeUndefined();
  expect(leerFecha('mañana')).toBeUndefined();
});

test('los adjuntos van a la columna sobre la que caen o a la primera de adjuntos', () => {
  const campos = [NOM, campo('a1', 'attachment'), campo('a2', 'attachment')];
  expect(campoDeAdjuntos(campos, 2)?.id).toBe('a2');
  expect(campoDeAdjuntos(campos, 0)?.id).toBe('a1');
  expect(campoDeAdjuntos(campos, -1)?.id).toBe('a1');
  expect(campoDeAdjuntos([NOM], 0)).toBeNull();
});

test('un CSV se reconoce por su extensión', () => {
  expect(esCSV('Proveedores 2026.CSV')).toBe(true);
  expect(esCSV('factura.csv.pdf')).toBe(false);
});
