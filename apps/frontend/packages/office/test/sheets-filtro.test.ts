// Checks de los filtros por condición y las vistas de filtro (sheets/filtro.ts).
// Ejecutar: `bun test`. Es el espejo de TestPasaCondicion de services/docs: si una
// semántica cambia, cambia la otra.
import { expect, test } from 'bun:test';
import type { CellValue } from '@sytue/formula';
import type { FilterCondition, SheetAutoFilter } from '../src/editor/types';
import {
  evalFilterColumn,
  filterViewsKey,
  hiddenByFilter,
  parseFilterViews,
  sanitizeFilterCond,
  sanitizeFilterConds,
  setCond,
  setExcluded,
  toggleValue,
  viewFromFilter,
  withView,
} from '../src/editor/sheets/filtro';

const col = (...vs: CellValue[]) => vs.map((v) => ({ v }));
const mixta = col(10, 20, 30, 'Sur', '', 40);
const pasa = (cond: FilterCondition, cells = mixta) => evalFilterColumn(cond, cells);

test('mayor/menor/igual/distinto sobre números y texto', () => {
  expect(pasa({ op: 'gt', a: '20' })).toEqual([false, false, true, false, false, true]);
  expect(pasa({ op: 'ge', a: '20' })).toEqual([false, true, true, false, false, true]);
  expect(pasa({ op: 'eq', a: 'sur' })).toEqual([false, false, false, true, false, false]);
  expect(pasa({ op: 'ne', a: '20' })).toEqual([true, false, true, true, true, true]);
});

test('«A1» no es un número: «igual a A1» compara texto', () => {
  expect(pasa({ op: 'eq', a: 'A1' }, col(1, 'a1'))).toEqual([false, true]);
});

test('entre admite los extremos al revés; «no entre» ignora el texto', () => {
  expect(pasa({ op: 'between', a: '30', b: '10' })).toEqual([
    true,
    true,
    true,
    false,
    false,
    false,
  ]);
  expect(pasa({ op: 'notBetween', a: '15', b: '35' })).toEqual([
    true,
    false,
    false,
    false,
    false,
    true,
  ]);
});

test('contiene, vacías y no vacías', () => {
  expect(pasa({ op: 'contains', a: 'u' })).toEqual([false, false, false, true, false, false]);
  expect(pasa({ op: 'blank' })).toEqual([false, false, false, false, true, false]);
  expect(pasa({ op: 'notBlank' })).toEqual([true, true, true, true, false, true]);
});

test('N superiores/inferiores, en porcentaje y con empates; sobre y bajo la media', () => {
  expect(pasa({ op: 'top', n: 2 })).toEqual([false, false, true, false, false, true]);
  expect(pasa({ op: 'bottom', n: 50, percent: true })).toEqual([
    true,
    true,
    false,
    false,
    false,
    false,
  ]);
  expect(pasa({ op: 'top', n: 1 }, col(5, 5, 1))).toEqual([true, true, false]);
  expect(pasa({ op: 'aboveAvg' })).toEqual([false, false, true, false, false, true]);
  expect(pasa({ op: 'belowAvg' })).toEqual([true, true, false, false, false, false]);
});

test('por color de relleno y de texto (el manual de la celda)', () => {
  const cells = [
    { v: 1 as CellValue, style: { fill: '#FF0000' } },
    { v: 2 as CellValue, style: { color: '#ff0000' } },
    { v: 3 as CellValue },
  ];
  expect(pasa({ op: 'fill', color: '#ff0000' }, cells)).toEqual([true, false, false]);
  expect(pasa({ op: 'fontColor', color: '#ff0000' }, cells)).toEqual([false, true, false]);
});

test('sanea condiciones de datos no confiables', () => {
  expect(sanitizeFilterCond({ op: 'gt', a: '5' })).toEqual({ op: 'gt', a: '5' });
  expect(sanitizeFilterCond({ op: 'gt' })).toBeNull(); // falta el operando
  expect(sanitizeFilterCond({ op: 'between', a: '1' })).toBeNull();
  expect(sanitizeFilterCond({ op: 'nope', a: '1' })).toBeNull();
  expect(sanitizeFilterCond({ op: 'top', n: 0 })).toBeNull();
  expect(sanitizeFilterCond({ op: 'top', n: 3, percent: true, a: 'x' })).toEqual({
    op: 'top',
    n: 3,
    percent: true,
  });
  expect(sanitizeFilterCond({ op: 'top', n: 300, percent: true })).toEqual({ op: 'top', n: 300 });
  expect(sanitizeFilterCond({ op: 'fill', color: 'rojo' })).toBeNull();
  expect(sanitizeFilterCond({ op: 'fill', color: '#AA0000' })).toEqual({
    op: 'fill',
    color: '#aa0000',
  });
  expect(sanitizeFilterConds({ '1': { op: 'blank' }, '9': { op: 'blank' } }, 0, 3)).toEqual({
    '1': { op: 'blank' },
  });
  expect(sanitizeFilterConds({ '9': { op: 'blank' } }, 0, 3)).toBeUndefined();
});

const porNumero = (a: number, b: number): number => a - b;

const src = (rows: CellValue[][]) => ({
  value: (c: number, r: number) => rows[r]?.[c] ?? '',
  style: () => undefined,
});

test('filas ocultas: valores excluidos y condiciones se intersecan, y las protegidas no se ocultan', () => {
  const rows: CellValue[][] = [
    ['Región', 'Importe'],
    ['Norte', 100],
    ['Sur', 250],
    ['Norte', 80],
    ['Este', 400],
    ['Total', 0],
  ];
  const af: SheetAutoFilter = {
    row: 0,
    c1: 0,
    c2: 1,
    excl: { '0': ['Sur'] },
    cond: { '1': { op: 'gt', a: '90' } },
  };
  expect([...hiddenByFilter(af, src(rows), rows.length)].sort(porNumero)).toEqual([2, 3, 5]);
  expect([...hiddenByFilter(af, src(rows), rows.length, new Set([5]))].sort(porNumero)).toEqual([
    2, 3,
  ]);
  expect(hiddenByFilter(undefined, src(rows), rows.length).size).toBe(0);
  // Fuera del rango [c1, c2] del autofiltro, una condición no filtra.
  expect([...hiddenByFilter({ ...af, c2: 0 }, src(rows), rows.length)]).toEqual([2]);
});

test('valores y condición de una columna se excluyen entre sí', () => {
  const base = { excl: { '1': ['x'] }, cond: { '2': { op: 'blank' as const } } };
  expect(setCond(base, 1, { op: 'gt', a: '3' })).toEqual({
    cond: { '1': { op: 'gt', a: '3' }, '2': { op: 'blank' } },
  });
});

test('toggleValue y setExcluded dejan JSON mínimo', () => {
  const a = toggleValue({}, 0, 'Sur', true);
  expect(a).toEqual({ excl: { '0': ['Sur'] } });
  expect(toggleValue(a, 0, 'Sur', false)).toEqual({});
  const conCond = setCond({}, 0, { op: 'blank' });
  expect(setExcluded(conCond, 0, ['x'])).toEqual({ excl: { '0': ['x'] } }); // excluir quita la condición
  expect(setCond(conCond, 0, null)).toEqual({});
});

test('una vista no toca el autofiltro compartido y lo sustituye solo al verse', () => {
  const af: SheetAutoFilter = { row: 2, c1: 0, c2: 3, excl: { '0': ['Sur'] } };
  const antes = structuredClone(af);
  const vista = viewFromFilter('v1', '  Solo Norte ', { cond: { '1': { op: 'gt', a: '5' } } });
  expect(vista.name).toBe('Solo Norte');
  expect(withView(af, vista)).toEqual({
    row: 2,
    c1: 0,
    c2: 3,
    cond: { '1': { op: 'gt', a: '5' } },
  });
  expect(withView(af, null)).toBe(af);
  expect(af).toEqual(antes);
});

test('las vistas se guardan por usuario y documento, con tolerancia a basura', () => {
  expect(filterViewsKey('u1', 'd1')).not.toBe(filterViewsKey('u2', 'd1'));
  expect(filterViewsKey('u1', 'd1')).not.toBe(filterViewsKey('u1', 'd2'));
  expect(filterViewsKey(undefined, 'd1')).toContain('anon');
  expect(parseFilterViews(null)).toEqual({ views: {}, active: {} });
  expect(parseFilterViews('{no es json')).toEqual({ views: {}, active: {} });
  expect(parseFilterViews('[1,2]')).toEqual({ views: {}, active: {} });
  const ok = parseFilterViews(
    JSON.stringify({
      views: {
        Hoja1: [
          { id: 'a', name: 'Norte', excl: { '0': ['Sur'] }, cond: { '1': { op: 'blank' } } },
          { id: 'b', name: '   ' }, // sin nombre: fuera
          { name: 'sin id' },
          { id: 'c', name: 'X', cond: { '1': { op: 'gt' } } }, // condición inválida: se descarta
        ],
        Hoja2: 'basura',
      },
      active: { Hoja1: 'a', Hoja2: 'zzz' },
    }),
  );
  expect(ok.views['Hoja1']).toEqual([
    { id: 'a', name: 'Norte', excl: { '0': ['Sur'] }, cond: { '1': { op: 'blank' } } },
    { id: 'c', name: 'X' },
  ]);
  expect(ok.views['Hoja2']).toBeUndefined();
  expect(ok.active).toEqual({ Hoja1: 'a' }); // la activa que no existe se olvida
});
