/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de la segmentación de datos (slicers):
 * valores únicos + estado de exclusión, traducción selección → conjunto excluido
 * y saneado del Slicer persistido.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  excludedFromSelection,
  pivotOnSlicerRemoval,
  pivotSlicerRef,
  pivotWithHidden,
  pivotWithoutHiddenFilter,
  sanitizeSlicer,
  slicerEntries,
} from '../src/editor/sheets/slicer';
import { computePivotTable, type PivotDef, type PivotLabels } from '../src/editor/sheets/pivot';

test('slicerEntries: únicos, orden es-ES y «(Vacías)» al final; marca los excluidos', () => {
  const e = slicerEntries(['Sur', 'Norte', '', 'Norte', 'Este'], ['Este']);
  expect(e.map((x) => x.value)).toEqual(['Este', 'Norte', 'Sur', '']); // alfabético es-ES, vacío al final
  expect(e.find((x) => x.value === 'Este')?.hidden).toBe(true);
  expect(e.find((x) => x.value === 'Norte')?.hidden).toBe(false);
});

test('slicerEntries: orden numérico (no lexicográfico)', () => {
  expect(slicerEntries(['10', '2', '1'], []).map((x) => x.value)).toEqual(['1', '2', '10']);
});

test('excludedFromSelection: complemento respecto a los únicos, en su orden', () => {
  expect(excludedFromSelection(['a', 'b', 'c'], ['a', 'c'])).toEqual(['b']);
  expect(excludedFromSelection(['a', 'b'], ['a', 'b'])).toEqual([]); // todo seleccionado → nada excluido
  expect(excludedFromSelection(['a', 'b'], [])).toEqual(['a', 'b']); // nada seleccionado → todo excluido
});

test('sanitizeSlicer: col válida + x/y acotadas; basura → null; id regenerado', () => {
  expect(sanitizeSlicer({ id: 's1', col: 2, x: 100, y: 50 })).toEqual({
    id: 's1',
    col: 2,
    x: 100,
    y: 50,
  });
  // La columna AD existe (la hoja llega a XFD); la 16.384 ya no.
  expect(sanitizeSlicer({ id: 's', col: 29 })).toEqual({ id: 's', col: 29 });
  expect(sanitizeSlicer({ col: 16_384 })).toBeNull();
  expect(sanitizeSlicer({ col: -1 })).toBeNull();
  expect(sanitizeSlicer(null)).toBeNull();
  const gen = sanitizeSlicer({ col: 0, x: -5, y: 9999 });
  expect(gen?.col).toBe(0);
  expect(gen?.x).toBe(0); // acotado a [0, 2200]
  expect(gen?.y).toBe(2200);
  expect(typeof gen?.id).toBe('string'); // id ausente → regenerado
});

test('segmentación de una tabla dinámica: el campo se busca en la tabla y comparte su hidden', () => {
  const pv: PivotDef = {
    rows: [{ field: 0 }],
    cols: [{ field: 1, hidden: ['x'] }],
    values: [{ field: 2, fn: 'sum' }],
    filters: [{ field: 1 }],
  };
  // Filtros antes que filas y columnas; un campo que no está en la tabla, uno nuevo.
  expect(pivotSlicerRef(pv, 1)).toEqual({ field: 1 });
  expect(pivotSlicerRef(pv, 0)).toEqual({ field: 0 });
  expect(pivotSlicerRef(pv, 2)).toEqual({ field: 2 });
  // Ocultar pone el mismo hidden en todos los sitios del campo; nada oculto lo quita.
  const oculto = pivotWithHidden(pv, 1, ['Sur']);
  expect(oculto.cols).toEqual([{ field: 1, hidden: ['Sur'] }]);
  expect(oculto.filters).toEqual([{ field: 1, hidden: ['Sur'] }]);
  expect(oculto.rows).toEqual([{ field: 0 }]);
  expect(pivotWithHidden(pv, 1, []).cols).toEqual([{ field: 1 }]);
  expect(pv.cols).toEqual([{ field: 1, hidden: ['x'] }]); // sin tocar el original
  expect(sanitizeSlicer({ id: 's', col: 1, pivot: 'TablaDinámica1' })?.pivot).toBe(
    'TablaDinámica1',
  );
  expect(sanitizeSlicer({ id: 's', col: 1, pivot: '' })).toEqual({ id: 's', col: 1 });
});

test('segmentación de un campo que no está en la tabla dinámica: su filtro oculto', () => {
  const pv: PivotDef = { rows: [{ field: 0 }], cols: [], values: [{ field: 2, fn: 'sum' }] };
  // Ocultar un campo de fuera de la tabla lo pone en hiddenFilters; nada oculto lo quita.
  const oculto = pivotWithHidden(pv, 1, ['Mesas']);
  expect(oculto.hiddenFilters).toEqual([{ field: 1, hidden: ['Mesas'] }]);
  expect(oculto.rows).toEqual([{ field: 0 }]);
  expect(pivotSlicerRef(oculto, 1)).toEqual({ field: 1, hidden: ['Mesas'] });
  expect(pivotWithHidden(oculto, 1, []).hiddenFilters).toBeUndefined();
  // Quitar la segmentación se lleva el filtro oculto; sin él, no hay nada que hacer.
  expect(pivotWithoutHiddenFilter(oculto, 1)?.hiddenFilters).toBeUndefined();
  expect(pivotWithoutHiddenFilter(pv, 1)).toBeNull();
  // Si el campo entra luego en la tabla, manda el de la tabla y el oculto se va al elegir.
  const dentro: PivotDef = { ...oculto, cols: [{ field: 1 }] };
  expect(pivotSlicerRef(dentro, 1)).toEqual({ field: 1 });
  const elegido = pivotWithHidden(dentro, 1, ['Sillas']);
  expect(elegido.cols).toEqual([{ field: 1, hidden: ['Sillas'] }]);
  expect(elegido.hiddenFilters).toBeUndefined();

  // Y filtra de verdad: sin pintarse, la tabla deja fuera las mesas.
  const L: PivotLabels = {
    blank: '(en blanco)',
    grand: 'Total general',
    totalOf: (x) => `Total ${x}`,
    valueName: (fn, f) => `${fn} de ${f}`,
    all: '(Todos)',
    multiple: '(Varios)',
    quarter: (q) => `T${q}`,
    month: (m) => `M${m}`,
    day: (n) => `D${n}`,
    show: (v) => String(v),
  };
  const grid = [
    ['Zona', 'Producto', 'Importe'],
    ['Norte', 'Sillas', 150],
    ['Norte', 'Mesas', 300],
    ['Sur', 'Mesas', 600],
    ['Sur', 'Sillas', 200],
  ];
  const valores = (g: ReturnType<typeof computePivotTable>): unknown[][] =>
    g.map((f) => f.map((c) => c?.v ?? null));
  expect(valores(computePivotTable(grid, oculto, L))).toEqual([
    ['Zona', 'sum de Importe'],
    ['Norte', 150],
    ['Sur', 200],
    ['Total general', 350],
  ]);
  // Dormido: con el campo en la tabla y sin nada oculto ahí, no filtra.
  expect(
    valores(computePivotTable(grid, dentro, L))
      .at(-1)
      ?.at(-1),
  ).toBe(1250);
});

test('quitar la segmentación de un campo de fuera de la tabla: se lleva su filtro, o no se quita', () => {
  const pv = {
    name: 'T1',
    rows: [{ field: 0 }],
    cols: [],
    values: [{ field: 2, fn: 'sum' as const }],
    hiddenFilters: [{ field: 1, hidden: ['Mesas'] }],
  };
  const fuera = { id: 's', col: 1, pivot: 'T1' };
  // Sin proteger, la tabla se reescribe sin el filtro oculto.
  const sin = pivotOnSlicerRemoval(fuera, [pv], false);
  expect(sin !== null && sin !== 'protected' && sin.hiddenFilters).toBeUndefined();
  // Protegida, no se puede reescribir: la tarjeta no se quita (y se avisa).
  expect(pivotOnSlicerRemoval(fuera, [pv], true)).toBe('protected');
  // La de un campo de la tabla, la del autofiltro o la de una tabla que ya no está se
  // quitan sin tocar nada, aunque la hoja esté protegida.
  expect(pivotOnSlicerRemoval({ id: 's', col: 0, pivot: 'T1' }, [pv], true)).toBeNull();
  expect(pivotOnSlicerRemoval({ id: 's', col: 1 }, [pv], true)).toBeNull();
  expect(pivotOnSlicerRemoval({ ...fuera, pivot: 'T2' }, [pv], true)).toBeNull();
});
