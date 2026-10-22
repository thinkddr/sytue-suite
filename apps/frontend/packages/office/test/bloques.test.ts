/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bloques reutilizables: colocar uno de hoja en la celda activa (referencias, fórmulas y
 * gráfico desplazados, lo de fuera saneado) y convertir uno de presentación en una
 * diapositiva con ids nuevos.
 *   cd apps/frontend && bun test packages/office/test/bloques.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { colocarBloque, MAX_CELDAS_BLOQUE } from '../src/editor/sheets/bloque';
import { diapositivaDeBloque } from '../src/editor/slides/bloque';
import type { SheetData } from '../src/editor/types';

const hoja = (): SheetData => ({ name: 'H', cells: { A1: 'ya estaba', D7: 'se pisa' } });

const TOTALES = JSON.stringify({
  cells: { A1: 'Subtotal', A2: 'IVA', B2: '0.21', C2: '=C1*B2', A3: '<b>Total</b>', C3: '=C1+C2' },
  formats: { C3: { id: 'currency', decimals: 2 }, B2: { id: 'no-existe' } },
  styles: { A3: { fill: '#B45309', color: '#FFFFFF', tema: 'marca', onclick: 'x' } },
  charts: [{ type: 'bar', range: 'A1:C3' }, { type: 'inventado', range: 'A1:B2' }, { type: 'bar' }],
});

describe('bloque de hoja', () => {
  test('se coloca en la celda activa y desplaza fórmulas, formatos, estilos y gráfico', () => {
    const p = colocarBloque(hoja(), TOTALES, { c: 3, r: 5 })!; // D6
    expect(p.cells!['D6']).toBe('Subtotal');
    expect(p.cells!['F7']).toBe('=F6*E7');
    expect(p.cells!['F8']).toBe('=F6+F7');
    expect(p.cells!['D8']).toBe('<b>Total</b>');
    expect(p.cells!['A1']).toBe('ya estaba'); // lo de fuera del bloque no se toca
    expect(p.cells!['D7']).toBe('IVA'); // lo de dentro se pisa, como un pegado
    expect(p.formats!['F8']).toEqual({ id: 'currency', decimals: 2 });
    expect(p.formats!['E7']).toBeUndefined(); // un formato desconocido no entra
    expect(p.styles!['D8']).toMatchObject({ fill: '#B45309', color: '#FFFFFF', tema: 'marca' });
    expect((p.styles!['D8'] as Record<string, unknown>)['onclick']).toBeUndefined();
    expect(p.charts).toHaveLength(1); // solo el gráfico válido
    expect(p.charts![0]!.range).toBe('D6:F8');
  });

  test('un bloque que no empieza en A1 se mide desde su propia esquina', () => {
    const p = colocarBloque(hoja(), JSON.stringify({ cells: { C5: '1', D6: '=C5*2' } }), {
      c: 0,
      r: 0,
    })!;
    expect(p.cells!['A1']).toBe('1');
    expect(p.cells!['B2']).toBe('=A1*2');
  });

  test('lo que no se entiende no se inserta', () => {
    const en = { c: 0, r: 0 };
    expect(colocarBloque(hoja(), 'no es json', en)).toBeNull();
    expect(colocarBloque(hoja(), '{"cells":{}}', en)).toBeNull();
    expect(colocarBloque(hoja(), '{"cells":{"A1:B2":"x"}}', en)).toBeNull();
    expect(colocarBloque(hoja(), '{"cells":{"no-ref":"x"}}', en)).toBeNull();
    // No cabe: la última columna de la rejilla no admite un bloque de dos de ancho.
    expect(colocarBloque(hoja(), '{"cells":{"A1":"a","B1":"b"}}', { c: 16_383, r: 0 })).toBeNull();
    const enorme = Object.fromEntries(
      Array.from({ length: MAX_CELDAS_BLOQUE + 1 }, (_, i) => [`A${i + 1}`, 'x']),
    );
    expect(colocarBloque(hoja(), JSON.stringify({ cells: enorme }), en)).toBeNull();
  });
});

describe('bloque de presentación', () => {
  const AGENDA = JSON.stringify({
    id: 'b',
    boxes: [
      { id: 'b-titulo', x: 8, y: 8, w: 84, h: 16, html: '<h1>Agenda</h1>' },
      { id: 'b-cuerpo', x: 8, y: 30, w: 84, h: 60, html: '<ol><li>Uno</li></ol>' },
      { id: 'rota', x: 'no', y: 0, w: 1, h: 1 },
      'basura',
    ],
    connectors: [{ from: 'b-titulo', to: 'b-cuerpo' }],
  });

  test('sale una diapositiva con ids nuevos y solo las cajas válidas', () => {
    const a = diapositivaDeBloque(AGENDA)!;
    const b = diapositivaDeBloque(AGENDA)!;
    expect(a.boxes.map((x) => x.html)).toEqual(['<h1>Agenda</h1>', '<ol><li>Uno</li></ol>']);
    const ids = [a.id, ...a.boxes.map((x) => x.id), b.id, ...b.boxes.map((x) => x.id)];
    expect(new Set(ids).size).toBe(ids.length); // insertar dos veces no repite ids
    expect(ids).not.toContain('b-titulo');
    expect(a.connectors).toBeUndefined();
  });

  test('lo que no es una diapositiva no se inserta', () => {
    expect(diapositivaDeBloque('x')).toBeNull();
    expect(diapositivaDeBloque('[]')).toBeNull();
    expect(diapositivaDeBloque('{"boxes":[]}')).toBeNull();
    expect(diapositivaDeBloque('{"boxes":"no"}')).toBeNull();
  });
});
