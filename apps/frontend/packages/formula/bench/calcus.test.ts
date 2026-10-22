/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los escenarios del benchmark (`bench/calcus.ts`) en pequeño: aquí no se mide tiempo
 * —depende de la máquina— sino que lo que mide el benchmark es correcto y que una
 * edición solo reevalúa a sus dependientes, que es lo que el benchmark da por supuesto.
 */
import { expect, test } from 'bun:test';
import { cargarHoja, leerTodo, medirTamano } from './calcus.js';

const N = 400; // 100 filas × 4 columnas

test('la hoja de referencia calcula lo que dice su cabecera', () => {
  const { engine: e, filas } = cargarHoja(N);
  expect(filas).toBe(100);
  leerTodo(e, filas);
  // D es el acumulado de B: la última fila es la suma de toda la columna.
  expect(e.getValue('Hoja1', `D${filas}`)).toBe(e.getValue('Hoja1', 'F1') as number);
  expect(e.getValue('Hoja1', 'B3')).toBeCloseTo(((3 * 37) % 1000) * 1.21, 9);
});

test('una edición suelta no reevalúa nada y una en A1 reevalúa la cadena entera', () => {
  const { engine: e, filas } = cargarHoja(N);
  leerTodo(e, filas);
  const base = e.evaluaciones;
  e.setCell('Hoja1', 'H1', 'nota');
  leerTodo(e, filas);
  expect(e.evaluaciones - base).toBe(0);

  const antes = e.evaluaciones;
  e.setCell('Hoja1', 'A1', '999');
  leerTodo(e, filas);
  // B1, C1, toda la columna D y los cuatro resúmenes; nada de las otras filas de B y C.
  expect(e.evaluaciones - antes).toBe(2 + filas + 4);
});

test('medirTamano devuelve los escenarios con las evaluaciones esperadas', () => {
  const m = Object.fromEntries(medirTamano(N, 1).map((x) => [x.escenario, x]));
  expect(Object.keys(m).sort()).toEqual(
    [
      'calculo-completo',
      'carga',
      'edicion-cascada',
      'edicion-cascada-completa',
      'edicion-con-volatil',
      'edicion-con-volatiles-muchas',
      'edicion-suelta',
      'recalculo-completo',
    ].sort(),
  );
  expect(m['edicion-suelta']!.evaluadas).toBe(0);
  expect(m['edicion-con-volatil']!.evaluadas).toBe(1); // solo el NOW()
  expect(m['recalculo-completo']!.evaluadas).toBe(3 * 100 + 4);
  expect(m['calculo-completo']!.evaluadas).toBe(3 * 100 + 4);
  // 4 volátiles (N/100): las únicas que se reevalúan al editar una celda suelta.
  expect(m['edicion-con-volatiles-muchas']!.evaluadas).toBe(4);
  expect(m['calculo-completo']!.cpuMs).toBeGreaterThanOrEqual(0);
  // La cascada reevalúa B1, C1, la columna D entera y los cuatro resúmenes.
  expect(m['edicion-cascada-completa']!.evaluadas).toBe(2 + 100 + 4);
  // Leyendo solo la ventana no se evalúa más que leyéndolo todo.
  expect(m['edicion-cascada']!.evaluadas).toBeLessThanOrEqual(
    m['edicion-cascada-completa']!.evaluadas,
  );
});
