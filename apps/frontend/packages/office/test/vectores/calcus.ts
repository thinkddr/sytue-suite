/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * VECTORES COMPARTIDOS de los objetos de Calcus (tablas con formato, referencias
 * estructuradas, minigráficos…) entre la web y la app nativa. Aquí están las ENTRADAS;
 * las salidas las calculan las funciones puras de la web y se guardan en
 * `calcus.json`, que lee también `apps/fyne/calcus/vectores_test.go`. Así el mismo
 * caso da lo mismo en los dos lados, y si uno cambia, su test lo dice.
 *
 * Regenerar (solo tras un cambio DELIBERADO de comportamiento):
 *   cd apps/frontend && bun packages/office/test/vectores/calcus.ts --escribir
 */
import { Engine, FormulaError } from '@sytue/formula';
import type { CellValue, TableArea } from '@sytue/formula';
import {
  collectResolvableTables,
  compileTables,
  expandRangeForEdit,
  resolveStructuredRef,
  sanitizeTable,
  tableCellInfo,
  tableTotalValue,
  uniqueHeaderNames,
  uniqueTableName,
} from '../../src/editor/sheets/table';
import type { TableTotalFn } from '../../src/editor/types';
import { extractRefs, tableBracketAt } from '../../src/editor/sheets/formula-edit';
import {
  sanitizeChartSpec,
  sanitizeSparkline,
  sparklineGeom,
} from '../../src/editor/sheets/format';
import { chartView } from '../../src/editor/sheets/Chart';
import type { ChartSpec, SheetData, Sparkline, SheetTable } from '../../src/editor/types';
import { moverObjetos } from '../../src/editor/sheets/refshift';
import type { Desplazamiento } from '../../src/editor/sheets/refshift';

/** Un valor de celda en JSON: los errores como `{ error: '#DIV/0!' }`. */
type ValorJSON = number | string | boolean | { error: string };
const aValor = (v: ValorJSON): CellValue =>
  typeof v === 'object' ? new FormulaError(v.error as FormulaError['code']) : v;
const deValor = (v: CellValue): ValorJSON => (v instanceof FormulaError ? { error: v.code } : v);

/* ---------- Entradas ---------- */

/* ---------- Funciones de hoja (el motor de la web y el de Go dan lo mismo) ---------- */

/** Datos de la Hoja1 que leen las fórmulas de `FUNCIONES` (referencia → número). */
const DATOS_FUNCIONES: Record<string, number> = (() => {
  const d: Record<string, number> = {};
  const r3 = (x: number): number => Math.round(x * 1000) / 1000;
  /** Ruido fijo en [-0,5, 0,5): sin azar, para que el JSON sea reproducible. */
  const ruido = (i: number): number => ((i * 7919) % 101) / 101 - 0.5;
  const poner = (col: string, v: number[]): void => v.forEach((x, i) => (d[`${col}${i + 1}`] = x));
  const t36 = Array.from({ length: 36 }, (_, i) => i + 1);
  // A, B: aditiva con periodo 6.
  poner('A', t36);
  poner(
    'B',
    t36.map((t) => r3(50 + 0.5 * t + 8 * Math.sin((2 * Math.PI * t) / 6) + 1.6 * ruido(t))),
  );
  // C, D: mensual en fechas (el día 1 de cada mes desde 2023-01), periodo 12.
  const t30 = t36.slice(0, 30);
  poner(
    'C',
    t30.map(
      (i) =>
        Math.round(Date.UTC(2023 + Math.floor((i - 1) / 12), (i - 1) % 12, 1) / 86400000) + 25569,
    ),
  );
  poner(
    'D',
    t30.map((t) => r3(100 + 2 * t + 10 * Math.cos((2 * Math.PI * t) / 12) + 2 * ruido(t))),
  );
  // E, F: con huecos (faltan 6, 7 y 20) y el instante 10 tres veces.
  const e = [...t30.filter((t) => ![6, 7, 20].includes(t)), 10, 10];
  poner('E', e);
  poner(
    'F',
    e.map((t, k) => r3(30 + 1.5 * t + 3 * ruido(t + k * 13))),
  );
  // G, H: multiplicativa con periodo 4.
  const t32 = t36.slice(0, 32);
  poner('G', t32);
  poner(
    'H',
    t32.map((t) => r3((20 + 0.4 * t) * (1 + 0.3 * Math.sin((2 * Math.PI * t) / 4)) + ruido(t))),
  );
  return d;
})();

const fecha = (y: number, m: number, d: number): string => `DATE(${y},${m},${d})`;
const ODD_F = `${fecha(2008, 11, 11)},${fecha(2021, 3, 1)},${fecha(2008, 10, 15)},${fecha(2009, 3, 1)}`;
const ODD_L = `${fecha(2008, 2, 7)},${fecha(2008, 9, 15)},${fecha(2007, 10, 15)}`;

const ETS_FORMULAS: string[] = (() => {
  const f: string[] = [];
  const serie = (t: string, v: string, o1: number, o3: number, sea: number[]): void => {
    f.push(`=FORECAST.ETS.ADD(${o1},${v},${t})`, `=FORECAST.ETS.ADD(${o3},${v},${t},${sea[0]})`);
    f.push(`=FORECAST.ETS.MULT(${o1},${v},${t})`, `=FORECAST.ETS.MULT(${o3},${v},${t},${sea[1]})`);
    f.push(`=FORECAST.ETS.PI.ADD(${o1},${v},${t})`, `=FORECAST.ETS.PI.ADD(${o3},${v},${t},0.9)`);
    f.push(`=FORECAST.ETS.PI.MULT(${o1},${v},${t})`, `=FORECAST.ETS.SEASONALITY(${v},${t})`);
    for (let k = 1; k <= 8; k++) f.push(`=FORECAST.ETS.STAT.ADD(${v},${t},${k})`);
    for (const k of [1, 3, 4, 7]) f.push(`=FORECAST.ETS.STAT.MULT(${v},${t},${k})`);
  };
  serie('A1:A36', 'B1:B36', 37, 40, [0, 6]);
  serie('G1:G32', 'H1:H32', 33, 35, [4, 0]);
  const ult = Math.round(Date.UTC(2025, 5, 1) / 86400000) + 25569; // 2025-06-01
  const may = Math.round(Date.UTC(2025, 7, 1) / 86400000) + 25569; // 2025-08-01
  serie('C1:C30', 'D1:D30', ult, may, [12, 12]);
  // Huecos y duplicados: las siete agregaciones y los dos modos de completar.
  for (const agg of [0, 1, 2, 3, 4, 5, 6, 7])
    f.push(`=FORECAST.ETS.ADD(31,F1:F29,E1:E29,1,1,${agg})`);
  f.push('=FORECAST.ETS.ADD(33,F1:F29,E1:E29,0,0)', '=FORECAST.ETS.ADD(33,F1:F29,E1:E29,1,0,4)');
  f.push('=FORECAST.ETS.STAT.ADD(F1:F29,E1:E29,8)', '=FORECAST.ETS(33,F1:F29,E1:E29)');
  // Errores.
  f.push(
    '=FORECAST.ETS.ADD(40,B1:B36,A1:A35)',
    '=FORECAST.ETS.ADD(30,B1:B36,A1:A36)',
    '=FORECAST.ETS.ADD(40,B1:B36,A1:A36,-1)',
    '=FORECAST.ETS.ADD(40,B1:B36,A1:A36,1,2)',
    '=FORECAST.ETS.ADD(40,B1:B36,A1:A36,1,1,8)',
    '=FORECAST.ETS.CONFINT(40,B1:B36,A1:A36,1)',
    '=FORECAST.ETS.STAT(B1:B36,A1:A36,9)',
    '=FORECAST.ETS.ADD(40,B1:B2,A1:A2)',
  );
  return f;
})();

/** Fórmulas cuyo resultado se guarda: lo que la web calcula es lo que Go debe calcular. */
const FUNCIONES: string[] = [
  // Cupones irregulares: ejemplos de Microsoft, periodos largos y todas las bases.
  `=ODDFPRICE(${ODD_F},0.0785,0.0625,100,2,1)`,
  `=ODDFYIELD(${ODD_F},0.0575,84.5,100,2,0)`,
  `=ODDFPRICE(${fecha(2008, 11, 11)},${fecha(2021, 3, 1)},${fecha(2008, 1, 15)},${fecha(2009, 3, 1)},0.0785,0.0625,100,2,0)`,
  `=ODDFPRICE(${fecha(2008, 11, 11)},${fecha(2021, 3, 1)},${fecha(2008, 10, 15)},${fecha(2009, 3, 1)},0.0785,0.0625,100,2,3)`,
  `=ODDFYIELD(${fecha(2008, 11, 11)},${fecha(2021, 3, 1)},${fecha(2008, 1, 15)},${fecha(2009, 3, 1)},0.0785,101.5,100,2,1)`,
  `=ODDFPRICE(${fecha(2008, 11, 11)},${fecha(2021, 3, 1)},${fecha(2008, 10, 15)},${fecha(2009, 2, 15)},0.05,0.05,100,2,1)`,
  `=ODDLPRICE(${fecha(2008, 2, 7)},${fecha(2008, 6, 15)},${fecha(2007, 10, 15)},0.0375,0.0405,100,2,0)`,
  `=ODDLYIELD(${fecha(2008, 4, 20)},${fecha(2008, 6, 15)},${fecha(2007, 12, 24)},0.0375,99.875,100,2,0)`,
  ...[0, 1, 2, 3, 4].flatMap((b) => [
    `=ODDLPRICE(${ODD_L},0.0375,0.0405,100,2,${b})`,
    `=ODDLYIELD(${ODD_L},0.0375,98.5,100,2,${b})`,
  ]),
  // Suavizado exponencial: pronóstico, intervalo, periodo y estadísticas, con las series de arriba.
  ...ETS_FORMULAS,
  // El texto escrito como argumento de un agregado se convierte; el de una función, no.
  '=SUM(1,"2")',
  '=SUM(A1:A2,"2")',
  '=SUM("1"&"0",1)',
  '=SUM("a",1)',
  '=SUM("",1)',
  '=AVERAGE("5",3)',
  '=MAX("7",3)',
  '=PRODUCT("2",3)',
  '=COUNT("1",1,TRUE)',
  '=COUNT("a",1)',
  '=SUM(LEFT("12",1),1)',
  '=SUM(INDEX(A1:A3,3),1)',
  // BESSELK a precisión de doble.
  '=BESSELK(1.5,1)',
  '=BESSELK(0.1,0)',
  '=BESSELK(5,1)',
  '=BESSELK(10,2)',
  '=BESSELK(0,1)',
];

/** Calcula `FUNCIONES` con el motor de la web (reloj y azar fijos). */
function evaluarFunciones(): { f: string; sale: ValorJSON }[] {
  const e = new Engine({ now: () => new Date(Date.UTC(2026, 9, 4)), random: () => 0.5 });
  for (const [ref, v] of Object.entries(DATOS_FUNCIONES)) e.setCell('Hoja1', ref, String(v));
  FUNCIONES.forEach((f, i) => e.setCell('Hoja1', `Z${i + 1}`, f));
  return FUNCIONES.map((f, i) => ({ f, sale: deValor(e.getValue('Hoja1', `Z${i + 1}`)) }));
}

const SANEAR_TABLAS: unknown[] = [
  { name: ' Ventas ', range: 'A1:C5' },
  { name: 'T', range: '$B$2:D9', style: 'green', headers: false, totalsRow: true },
  { name: 'T', range: 'A1:B3', style: 'inexistente', headers: true, totalsRow: false },
  {
    name: 'T',
    range: 'A1:C4',
    totals: { '0': 'none', '1': 'sum', '2': 'nada', x: 'max', '-1': 'min', '1.5': 'sum' },
  },
  { name: '', range: 'A1:B2' },
  { name: 'T', range: 'no es un rango' },
  { name: 'T' },
  { range: 'A1:B2' },
  null,
  'texto',
];

const TOTALES: { fn: TableTotalFn; valores: ValorJSON[] }[] = [
  { fn: 'sum', valores: [10, 'texto', 20, '', 30] },
  { fn: 'average', valores: [10, 'texto', 20, '', 30] },
  { fn: 'min', valores: [10, 'texto', 20, '', 30] },
  { fn: 'max', valores: [10, 'texto', 20, '', 30] },
  { fn: 'count', valores: [1, 'a', '', 2, { error: '#DIV/0!' }] },
  { fn: 'counta', valores: [1, 'a', '', 2, { error: '#DIV/0!' }, true] },
  { fn: 'none', valores: [1, 2] },
  { fn: 'sum', valores: ['a', '', 'b'] },
  { fn: 'average', valores: ['a', '', 'b'] },
  { fn: 'max', valores: [] },
  { fn: 'count', valores: ['a'] },
  { fn: 'average', valores: [1, 2, 4] },
];

const CABECERAS: string[][] = [
  ['Región', '', 'Región', 'región', ' Coste ', 'Región2'],
  ['', '', ''],
  ['A', 'a', 'A2', 'A'],
];

const NOMBRES_LIBRES: string[][] = [[], ['Tabla1'], ['tabla1', 'TABLA2', 'Tabla4'], ['Ventas']];

const TABLAS_GEOMETRIA: SheetTable[][] = [
  [{ name: 'T', range: 'A1:B5', totalsRow: true }],
  [{ name: 'T', range: 'B2:D6', headers: false }],
  [
    { name: 'A', range: 'A1:C6' },
    { name: 'B', range: 'B3:D8' },
  ],
];
const CELDAS_GEOMETRIA: [number, number][] = [
  [0, 0],
  [0, 1],
  [0, 2],
  [1, 3],
  [1, 4],
  [2, 2],
  [3, 5],
  [2, 7],
  [5, 0],
];

const CRECER: { tabla: SheetTable; c: number; f: number }[] = [
  { tabla: { name: 'T', range: 'A1:C5' }, c: 3, f: 2 },
  { tabla: { name: 'T', range: 'A1:C5' }, c: 1, f: 5 },
  { tabla: { name: 'T', range: 'A1:C5', totalsRow: true }, c: 1, f: 5 },
  { tabla: { name: 'T', range: 'A1:C5' }, c: 4, f: 2 },
  { tabla: { name: 'T', range: 'A1:C5' }, c: 1, f: 2 },
  { tabla: { name: 'T', range: 'A1:Z5' }, c: 26, f: 2 },
  { tabla: { name: 'T', range: 'A1:C100' }, c: 1, f: 100 },
];

// El libro de las estructuradas: el de `sheets-structured.test.ts`.
const HOJAS = [
  { name: 'Hoja1', tables: [{ name: 'Ventas', range: 'A1:C5' }] as SheetTable[] },
  { name: 'Hoja2', tables: [{ name: 'Compras', range: 'B2:C4', totalsRow: true }] as SheetTable[] },
  { name: 'Hoja3', tables: [{ name: 'SinCab', range: 'A1:B3', headers: false }] as SheetTable[] },
];
const CABECERAS_LIBRO: Record<string, string> = {
  'Hoja1!0,0': 'Region',
  'Hoja1!1,0': 'Ventas',
  'Hoja1!2,0': 'Coste',
  'Hoja2!1,1': 'Fecha',
  'Hoja2!2,1': 'Total',
};
const cabecera = (hoja: string, c: number, r: number): string =>
  CABECERAS_LIBRO[`${hoja}!${c},${r}`] ?? '';

interface Consulta {
  table: string;
  column: string | undefined;
  area: TableArea;
  thisRow: boolean;
}
const q = (
  table: string,
  column?: string,
  area: TableArea = 'data',
  thisRow = false,
): Consulta => ({
  table,
  column,
  area,
  thisRow,
});
const DENTRO = { sheet: 'Hoja1', col: 1, row: 2 };
const RESOLVER: { q: Consulta; aqui: { sheet: string; col: number; row: number } }[] = [
  { q: q('Ventas', 'Ventas'), aqui: DENTRO },
  { q: q('ventas', 'coste'), aqui: DENTRO },
  { q: q('Ventas', 'Ventas', 'data', true), aqui: { sheet: 'Hoja1', col: 7, row: 3 } },
  { q: q('Ventas', 'Ventas', 'data', true), aqui: { sheet: 'Hoja1', col: 7, row: 0 } },
  { q: q('Ventas', 'Ventas', 'data', true), aqui: { sheet: 'Hoja2', col: 1, row: 3 } },
  { q: q('Ventas', 'Ventas', 'headers'), aqui: DENTRO },
  { q: q('Ventas', undefined, 'all'), aqui: DENTRO },
  { q: q('Ventas', undefined, 'totals'), aqui: DENTRO },
  { q: q('Compras', 'Total', 'totals'), aqui: DENTRO },
  { q: q('Compras', 'Fecha'), aqui: DENTRO },
  { q: q('Foo', 'Bar'), aqui: DENTRO },
  { q: q('Ventas', 'NoExiste'), aqui: DENTRO },
  { q: q('', 'Coste', 'data', true), aqui: DENTRO },
  { q: q('', 'Coste', 'data', true), aqui: { sheet: 'Hoja1', col: 9, row: 9 } },
  { q: q('SinCab', 'Columna2'), aqui: DENTRO },
  { q: q('SinCab', 'Columna1', 'headers'), aqui: DENTRO },
];

const CORCHETES: { texto: string; cursor: number }[] = [
  { texto: '=Ventas[Reg', cursor: 11 },
  { texto: '=Ventas[@V', cursor: 10 },
  { texto: '=Ventas[', cursor: 8 },
  { texto: '=[@Co', cursor: 5 },
  { texto: '=Ventas[Region]', cursor: 15 },
  { texto: '=Ventas[#Da', cursor: 11 },
  { texto: '=Ventas[[#Datos],[Re', cursor: 20 },
  { texto: '=SUM(A1)', cursor: 6 },
  { texto: 'Ventas[Reg', cursor: 10 },
  { texto: '=SUMA(Ventas[Coste])+Compras[To', cursor: 31 },
];

const REFS: string[] = [
  '=SUM(Ventas[Ventas])',
  '=A1+Ventas[Coste]',
  '="Ventas[Ventas]"',
  '=Ventas[[#Cabeceras],[Coste]]',
  '=B2:B5+Ventas[Ventas]*Hoja2!C3',
  '=Foo[Bar]+Ventas[Region]',
  '=Ventas[Coste',
];

const MINIS: { valores: number[]; tipo: Sparkline['type'] }[] = [
  { valores: [1, 3, 2, 5, 4], tipo: 'line' },
  { valores: [7], tipo: 'line' },
  { valores: [2, 2, 2], tipo: 'line' },
  { valores: [], tipo: 'line' },
  { valores: [1, -2, 0, 3], tipo: 'column' },
  { valores: [-1, -5], tipo: 'column' },
  { valores: [0, 0], tipo: 'column' },
  { valores: [1, -2, 0, 3, -0.5, 8, 2, 1, 1, 1, 4, -4, 5], tipo: 'winloss' },
];

const SANEAR_MINIS: unknown[] = [
  { cell: 'A1', range: 'B1:B9', type: 'line' },
  { cell: 'A1', range: 'B1:B9', type: 'column', color: '#16a34a' },
  { cell: 'A1', range: 'B1:B9', type: 'winloss', color: 'rojo' },
  { cell: 'A1', range: 'B1:B9' },
  { cell: 'nada', range: 'B1:B9', type: 'line' },
  { cell: 'A1', range: 'B1:B9', type: 'pie' },
  null,
];

/* ---------- Gráficos ---------- */

// La hoja de los gráficos: A1:E7 con cabecera, rótulos de mes, un negativo, un texto
// que cuenta 0 y una columna de X para la dispersión (E).
const HOJA_GRAFICOS: ValorJSON[][] = [
  ['Mes', 'Ventas', 'Coste', 'Margen', 'X'],
  ['Enero', 120, 80, 40, 1.5],
  ['Febrero', 95.5, 70, 25.5, 2],
  ['Marzo', 150, 90, -12, 3.25],
  ['Abril', 'n/d', 60, 30, 4],
  ['Mayo', 180, 100, 80, 5.5],
  ['Junio', 1234.5, 900, 334.5, 7],
];
const valorHoja = (c: number, r: number): CellValue => {
  const v = HOJA_GRAFICOS[r]?.[c];
  return v === undefined ? '' : aValor(v);
};

const GRAFICOS: ChartSpec[] = [
  { id: 'g', type: 'bar', range: 'A1:C7' },
  { id: 'g', type: 'bar', range: 'A1:D7', stacked: true, labels: true },
  { id: 'g', type: 'hbar', range: 'A1:C7', labels: true },
  { id: 'g', type: 'hbar', range: 'A1:D5', stacked: true, xTitle: 'Importe', yTitle: 'Mes' },
  { id: 'g', type: 'line', range: 'A1:D7', labels: true },
  { id: 'g', type: 'area', range: 'A1:C7' },
  { id: 'g', type: 'area', range: 'A1:D7', stacked: true },
  { id: 'g', type: 'combo', range: 'A1:D7', secondaryAxis: [2] },
  { id: 'g', type: 'combo', range: 'A1:D7', lineSeries: [0, 1] },
  { id: 'g', type: 'pie', range: 'A1:B7', labels: true },
  { id: 'g', type: 'donut', range: 'A1:B7', labels: true, colors: ['#16A34A', '', '#7C3AED'] },
  { id: 'g', type: 'scatter', range: 'E1:C7' },
  { id: 'g', type: 'scatter', range: 'E1:E7', labels: true },
  { id: 'g', type: 'radar', range: 'A1:C7' },
  { id: 'g', type: 'bar', range: 'A1:D3', byRow: true },
  {
    id: 'g',
    type: 'line',
    range: 'A1:C7',
    legend: 'right',
    w: 420,
    h: 240,
    grid: false,
    xTitle: 'Mes',
    yTitle: 'Euros',
  },
  { id: 'g', type: 'bar', range: 'A1:C7', legend: 'none', colors: ['', '#0D9488'] },
  { id: 'g', type: 'pie', range: 'A1:A1' },
  { id: 'g', type: 'bar', range: 'H1:I3' },
];

const SANEAR_GRAFICOS: unknown[] = [
  { id: 'c1', type: 'bar', range: 'A1:B4', x: 10.6, y: -5 },
  {
    id: 'c',
    type: 'hbar',
    range: 'A1:C9',
    stacked: true,
    labels: true,
    legend: 'right',
    grid: false,
  },
  {
    id: 'c',
    type: 'line',
    range: 'A1:C9',
    legend: 'bottom',
    grid: true,
    byRow: true,
    stacked: 'sí',
  },
  { id: 'c', type: 'bar', range: 'A1:C9', w: 90, h: 5000.4, xTitle: '  Mes  ', yTitle: 42 },
  { id: 'c', type: 'bar', range: 'A1:C9', colors: ['#16A34A', 'rojo', '', '#ABCDEF', ''] },
  { id: 'c', type: 'bar', range: 'A1:C9', colors: ['x', ''], legend: 'izquierda' },
  {
    id: 'c',
    type: 'combo',
    range: 'A1:C9',
    lineSeries: [2, 2, -1, 1.5, 30, 16384, 0],
    secondaryAxis: [],
  },
  { id: 'c', type: 'bogus', range: 'A1' },
  { id: 'c', type: 'bar', range: 'no-parsea' },
  null,
];

/* ---------- Mover los objetos al insertar o borrar filas y columnas ---------- */

const HOJA_OBJETOS: SheetData = {
  name: 'H',
  cells: {},
  tables: [
    { name: 'Ventas', range: 'B2:D6', totalsRow: true, totals: { '2': 'sum' } },
    { name: 'Chica', range: 'F10:G10' },
  ],
  sparklines: [
    { cell: 'H3', range: 'C3:D3', type: 'line' },
    { cell: 'A9', range: 'B9:D9', type: 'column' },
  ],
  charts: [
    { id: 'g1', type: 'bar', range: 'B2:D6', x: 300, y: 40 },
    { id: 'g2', type: 'pie', range: 'F10:G10' },
  ],
  slicers: [
    { id: 's1', col: 1 },
    { id: 's2', col: 3, x: 10, y: 10 },
    // La de una tabla dinámica no se mueve: su `col` es un campo del origen.
    { id: 's3', col: 1, pivot: 'TablaDinámica1' },
  ],
  autofilter: { row: 1, c1: 1, c2: 3, excl: { '1': ['Norte'], '3': ['0'] } },
};
const DESPLAZAMIENTOS: Desplazamiento[] = [
  { eje: 'fila', desde: 3, cantidad: 1, tope: 100 },
  { eje: 'fila', desde: 0, cantidad: 2, tope: 100 },
  { eje: 'fila', desde: 9, cantidad: -1, tope: 100 },
  { eje: 'fila', desde: 1, cantidad: -1, tope: 100 },
  { eje: 'fila', desde: 2, cantidad: -3, tope: 100 },
  { eje: 'columna', desde: 2, cantidad: 1, tope: 26 },
  { eje: 'columna', desde: 3, cantidad: -1, tope: 26 },
  { eje: 'columna', desde: 1, cantidad: -3, tope: 26 },
  { eje: 'columna', desde: 0, cantidad: 25, tope: 26 },
];

/* ---------- Cálculo ---------- */

/** Lo que dicen las funciones de la web de cada entrada. */
export function calcularVectores(): unknown {
  const libro = collectResolvableTables(HOJAS, cabecera);
  return {
    tablas: {
      sanear: SANEAR_TABLAS.map((entrada) => ({ entrada, sale: sanitizeTable(entrada) })),
      totales: TOTALES.map(({ fn, valores }) => ({
        fn,
        valores,
        sale: deValor(tableTotalValue(fn, valores.map(aValor))),
      })),
      cabeceras: CABECERAS.map((entrada) => ({ entrada, sale: uniqueHeaderNames(entrada) })),
      nombreLibre: NOMBRES_LIBRES.map((existentes) => ({
        existentes,
        sale: uniqueTableName(existentes),
      })),
      info: TABLAS_GEOMETRIA.flatMap((tablas) =>
        CELDAS_GEOMETRIA.map(([c, f]) => {
          const compiladas = compileTables(tablas);
          const i = tableCellInfo(compiladas, c, f);
          return {
            tablas,
            c,
            f,
            sale: i
              ? {
                  tabla: compiladas.indexOf(i.ct),
                  papel: i.role,
                  banda: i.banded,
                  col: i.colIndex,
                }
              : null,
          };
        }),
      ),
      crecer: CRECER.map(({ tabla, c, f }) => {
        const [ct] = compileTables([tabla]);
        return { tabla, c, f, sale: ct ? expandRangeForEdit(ct, c, f, 25, 99) : null };
      }),
      resolver: {
        hojas: HOJAS,
        cabeceras: CABECERAS_LIBRO,
        casos: RESOLVER.map(({ q: consulta, aqui }) => {
          const r = resolveStructuredRef(libro, consulta, aqui);
          return {
            q: { ...consulta, column: consulta.column ?? null },
            aqui,
            sale: r === undefined ? null : r instanceof FormulaError ? { error: r.code } : r,
          };
        }),
      },
      corchetes: CORCHETES.map(({ texto, cursor }) => ({
        texto,
        cursor,
        sale: tableBracketAt(texto, cursor),
      })),
      refs: REFS.map((texto) => ({
        texto,
        sale: extractRefs(texto, (spec) => {
          const r = resolveStructuredRef(libro, spec, DENTRO);
          if (!r || r instanceof FormulaError) return null;
          return { sheet: r.sheet, rect: { c1: r.c1, r1: r.r1, c2: r.c2, r2: r.r2 } };
        }),
      })),
    },
    graficos: {
      hoja: HOJA_GRAFICOS,
      vistas: GRAFICOS.map((spec) => ({ spec, sale: chartView(spec, valorHoja, 'Otros') })),
      sanear: SANEAR_GRAFICOS.map((entrada) => ({ entrada, sale: sanitizeChartSpec(entrada) })),
    },
    moverObjetos: {
      hoja: HOJA_OBJETOS,
      casos: DESPLAZAMIENTOS.map((d) => {
        const { tables, sparklines, charts, slicers, autofilter } = moverObjetos(HOJA_OBJETOS, d);
        return { d, sale: { tables, sparklines, charts, slicers, autofilter } };
      }),
    },
    funciones: { datos: DATOS_FUNCIONES, casos: evaluarFunciones() },
    minigraficos: {
      geometria: MINIS.map(({ valores, tipo }) => ({
        valores,
        tipo,
        sale: sparklineGeom(valores, tipo),
      })),
      sanear: SANEAR_MINIS.map((entrada) => ({ entrada, sale: sanitizeSparkline(entrada) })),
    },
  };
}

if (import.meta.main && process.argv.includes('--escribir')) {
  const ruta = new URL('./calcus.json', import.meta.url);
  await Bun.write(ruta, `${JSON.stringify(calcularVectores(), null, 1)}\n`);
  process.stdout.write(`escrito ${ruta.pathname}\n`);
}
