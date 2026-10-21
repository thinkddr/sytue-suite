/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Checks de las REFERENCIAS ESTRUCTURADAS de tabla `Tabla[Columna]` (paridad
 * Excel/Sheets): parseo (incl. `parseBracketSpec` puro), evaluación con un
 * resolutor de prueba y la taxonomía de errores. `bun test`.
 */
import { expect, test } from 'bun:test';
import { parseFormula, parseBracketSpec, type AstNode } from './parser.js';
import { Engine } from './engine.js';
import { ERR } from './values.js';
import type { TableResolver } from './registry.js';

/* ---------- Utilidades ---------- */

const structured = (n: AstNode): Extract<AstNode, { kind: 'structured' }> => {
  if (n.kind !== 'structured') throw new Error('no es structured: ' + n.kind);
  return n;
};

/**
 * Resolutor de prueba: modela UNA tabla «Ventas» en Hoja1 con cabecera en la
 * fila 0, datos en las filas 1-3 y totales en la fila 4 (0-based), columnas
 * Region/Ventas/Coste/Margen (A..D). Reproduce la semántica que el editor
 * implementará: #NAME? si la tabla no existe, #REF! si la columna no existe,
 * #VALUE! si `@` cae fuera del cuerpo de datos.
 */
const TABLE = {
  sheet: 'Hoja1',
  header: 0,
  firstData: 1,
  lastData: 3,
  totals: 4,
  c0: 0,
  cols: ['Region', 'Ventas', 'Coste', 'Margen'],
};

const resolver: TableResolver = (q, here) => {
  // Localizar la tabla (explícita por nombre, o implícita = la que cubre `here`).
  let def: typeof TABLE | undefined;
  if (q.table === '') {
    const inSpan =
      here.sheet === TABLE.sheet &&
      here.col >= TABLE.c0 &&
      here.col < TABLE.c0 + TABLE.cols.length &&
      here.row >= TABLE.header &&
      here.row <= TABLE.totals;
    if (!inSpan) return undefined; // fuera de toda tabla → #REF!
    def = TABLE;
  } else if (q.table.toLowerCase() === 'ventas') {
    def = TABLE;
  } else {
    return ERR.NAME; // tabla desconocida → #NAME?
  }

  // Columnas (todas si no se pide una).
  let c1 = def.c0;
  let c2 = def.c0 + def.cols.length - 1;
  if (q.column !== undefined) {
    const idx = def.cols.findIndex((n) => n.toLowerCase() === q.column!.toLowerCase());
    if (idx < 0) return ERR.REF; // columna inexistente → #REF!
    c1 = c2 = def.c0 + idx;
  }

  // Filas por área.
  let r1: number;
  let r2: number;
  switch (q.area) {
    case 'headers':
      r1 = r2 = def.header;
      break;
    case 'totals':
      r1 = r2 = def.totals;
      break;
    case 'all':
      r1 = def.header;
      r2 = def.totals;
      break;
    default: // 'data'
      r1 = def.firstData;
      r2 = def.lastData;
  }

  // `@` fila actual: intersecar con la fila de la celda; #VALUE! si está fuera.
  if (q.thisRow) {
    if (here.sheet !== def.sheet || here.row < def.firstData || here.row > def.lastData)
      return ERR.VALUE;
    r1 = r2 = here.row;
  }

  return { sheet: def.sheet, c1, r1, c2, r2 };
};

/** Motor con la tabla «Ventas» poblada y el resolutor de prueba conectado. */
const setup = (): Engine => {
  const e = new Engine({ resolveTable: resolver });
  const rows: [string, string, string][] = [
    ['Region', 'Ventas', 'Coste'],
    ['Norte', '10', '4'],
    ['Sur', '20', '9'],
    ['Este', '30', '12'],
    ['Total', '60', '25'],
  ];
  rows.forEach((r, i) => {
    e.setCell('Hoja1', `A${i + 1}`, r[0]);
    e.setCell('Hoja1', `B${i + 1}`, r[1]);
    e.setCell('Hoja1', `C${i + 1}`, r[2]);
  });
  e.setCell('Hoja1', 'D1', 'Margen'); // cabecera de la columna calculada
  return e;
};

/* ---------- Parseo ---------- */

test('parse: Tabla[Columna] → nodo structured (columna de datos)', () => {
  const n = structured(parseFormula('=Ventas[Ventas]'));
  expect(n.table).toBe('Ventas');
  expect(n.column).toBe('Ventas');
  expect(n.area).toBe('data');
  expect(n.thisRow).toBe(false);
});

test('parse: Tabla[@Columna] → fila actual', () => {
  const n = structured(parseFormula('=Ventas[@Ventas]'));
  expect(n.table).toBe('Ventas');
  expect(n.column).toBe('Ventas');
  expect(n.thisRow).toBe(true);
});

test('parse: [@Columna] y [Columna] → tabla implícita (table = "")', () => {
  const a = structured(parseFormula('=[@Coste]'));
  expect(a.table).toBe('');
  expect(a.column).toBe('Coste');
  expect(a.thisRow).toBe(true);
  const b = structured(parseFormula('=[Coste]'));
  expect(b.table).toBe('');
  expect(b.column).toBe('Coste');
  expect(b.thisRow).toBe(false);
});

test('parse: especificadores de área ES', () => {
  expect(structured(parseFormula('=Ventas[#Todo]')).area).toBe('all');
  expect(structured(parseFormula('=Ventas[#Datos]')).area).toBe('data');
  expect(structured(parseFormula('=Ventas[#Cabeceras]')).area).toBe('headers');
  expect(structured(parseFormula('=Ventas[#Totales]')).area).toBe('totals');
  // Solo área → sin columna.
  expect(structured(parseFormula('=Ventas[#Todo]')).column).toBeUndefined();
});

test('parse: especificadores de área EN', () => {
  expect(structured(parseFormula('=T[#All]')).area).toBe('all');
  expect(structured(parseFormula('=T[#Data]')).area).toBe('data');
  expect(structured(parseFormula('=T[#Headers]')).area).toBe('headers');
  expect(structured(parseFormula('=T[#Totals]')).area).toBe('totals');
});

test('parse: forma anidada [[#Datos],[Columna]] y [#Esta fila]', () => {
  const a = structured(parseFormula('=Ventas[[#Datos],[Ventas]]'));
  expect(a.area).toBe('data');
  expect(a.column).toBe('Ventas');
  const b = structured(parseFormula('=Ventas[[#Cabeceras],[Coste]]'));
  expect(b.area).toBe('headers');
  expect(b.column).toBe('Coste');
  const c = structured(parseFormula('=Ventas[[#Esta fila],[Ventas]]'));
  expect(c.thisRow).toBe(true);
  expect(c.column).toBe('Ventas');
});

test('parse: nombre de columna con espacios', () => {
  const n = structured(parseFormula('=Ventas[Precio Unitario]'));
  expect(n.column).toBe('Precio Unitario');
});

test('parse: structured dentro de aritmética y como argumento', () => {
  const mul = parseFormula('=Ventas[Ventas]*2');
  expect(mul.kind).toBe('binary');
  if (mul.kind === 'binary') expect(mul.left.kind).toBe('structured');
  const call = parseFormula('=SUM(Ventas[Ventas])');
  if (call.kind !== 'call') throw new Error('call');
  expect(call.args[0]!.kind).toBe('structured');
});

test('parse: retrocompat — rangos/llamadas/hoja siguen intactos', () => {
  expect(parseFormula('=A1:B10').kind).toBe('range');
  expect(parseFormula('=SUM(1,2,3)').kind).toBe('call');
  const s = parseFormula('=Hoja1!A1');
  if (s.kind !== 'ref') throw new Error('ref');
  expect(s.ref.sheet).toBe('Hoja1');
});

test('parse: «]» sin abrir es error de parseo', () => {
  expect(() => parseFormula('=A1]')).toThrow();
  expect(() => parseFormula('=Ventas[Ventas')).toThrow(); // corchete sin cerrar
});

/* ---------- parseBracketSpec (puro) ---------- */

test('parseBracketSpec: formas simples', () => {
  expect(parseBracketSpec('Ventas')).toEqual({ column: 'Ventas', area: 'data', thisRow: false });
  expect(parseBracketSpec('@Ventas')).toEqual({ column: 'Ventas', area: 'data', thisRow: true });
  expect(parseBracketSpec('#Todo')).toEqual({ column: undefined, area: 'all', thisRow: false });
  expect(parseBracketSpec('  Precio Unitario  ')).toEqual({
    column: 'Precio Unitario',
    area: 'data',
    thisRow: false,
  });
});

test('parseBracketSpec: anidado y combinado', () => {
  expect(parseBracketSpec('[#Totales],[Coste]')).toEqual({
    column: 'Coste',
    area: 'totals',
    thisRow: false,
  });
  expect(parseBracketSpec('[#Esta fila],[Ventas]')).toEqual({
    column: 'Ventas',
    area: 'data',
    thisRow: true,
  });
});

test('parseBracketSpec: el apóstrofo escapa un `@`/`#` literal en el nombre', () => {
  // `['@Total]` es una columna llamada literalmente «@Total», no la fila actual.
  expect(parseBracketSpec("'@Total")).toEqual({ column: '@Total', area: 'data', thisRow: false });
  expect(parseBracketSpec("'#raro")).toEqual({ column: '#raro', area: 'data', thisRow: false });
});

/* ---------- Evaluación ---------- */

test('eval: SUM sobre columna de datos entera', () => {
  const e = setup();
  e.setCell('Hoja1', 'F1', '=SUM(Ventas[Ventas])');
  expect(e.getValue('Hoja1', 'F1')).toBe(60);
  e.setCell('Hoja1', 'F2', '=SUM(Ventas[Coste])');
  expect(e.getValue('Hoja1', 'F2')).toBe(25);
});

test('eval: columna de datos DERRAMA (spill) hacia abajo', () => {
  const e = setup();
  e.setCell('Hoja1', 'F1', '=Ventas[Ventas]');
  expect(e.getValue('Hoja1', 'F1')).toBe(10);
  expect(e.getValue('Hoja1', 'F2')).toBe(20);
  expect(e.getValue('Hoja1', 'F3')).toBe(30);
});

test('eval: forma anidada, columna insensible a mayúsculas', () => {
  const e = setup();
  e.setCell('Hoja1', 'F1', '=SUM(Ventas[[#Datos],[ventas]])'); // «ventas» minúscula
  expect(e.getValue('Hoja1', 'F1')).toBe(60);
});

test('eval: áreas #Totales, #Cabeceras y #Todo', () => {
  const e = setup();
  e.setCell('Hoja1', 'F1', '=SUM(Ventas[#Totales])'); // fila 5: B5+C5 (A5 texto, D5 vacío)
  expect(e.getValue('Hoja1', 'F1')).toBe(85);
  e.setCell('Hoja1', 'F2', '=Ventas[[#Cabeceras],[Ventas]]');
  expect(e.getValue('Hoja1', 'F2')).toBe('Ventas');
  e.setCell('Hoja1', 'F3', '=SUM(Ventas[[#Todo],[Coste]])'); // C1..C5: 4+9+12+25 (C1 texto)
  expect(e.getValue('Hoja1', 'F3')).toBe(50);
});

test('eval: @ fila actual con tabla explícita', () => {
  const e = setup();
  e.setCell('Hoja1', 'F2', '=Ventas[@Ventas]'); // F2 → fila de datos 1 → B2 = 10
  expect(e.getValue('Hoja1', 'F2')).toBe(10);
  e.setCell('Hoja1', 'F3', '=Ventas[@Ventas]*2'); // fila 2 → B3 = 20 → 40
  expect(e.getValue('Hoja1', 'F3')).toBe(40);
});

test('eval: @ fila actual con tabla IMPLÍCITA (columna calculada)', () => {
  const e = setup();
  e.setCell('Hoja1', 'D2', '=[@Ventas]-[@Coste]'); // D2 en la tabla, fila 1 → 10-4
  expect(e.getValue('Hoja1', 'D2')).toBe(6);
});

test('eval: @ fuera del cuerpo de datos → #VALUE!', () => {
  const e = setup();
  e.setCell('Hoja1', 'F10', '=Ventas[@Ventas]'); // fila 9, fuera de 1..3
  expect(e.getDisplay('Hoja1', 'F10')).toBe('#VALUE!');
});

test('eval: tabla desconocida → #NAME?; columna desconocida → #REF!', () => {
  const e = setup();
  e.setCell('Hoja1', 'F1', '=SUM(Foo[Bar])');
  expect(e.getDisplay('Hoja1', 'F1')).toBe('#NAME?');
  e.setCell('Hoja1', 'F2', '=SUM(Ventas[NoExiste])');
  expect(e.getDisplay('Hoja1', 'F2')).toBe('#REF!');
});

test('eval: sin resolutor inyectado → toda referencia estructurada es #REF!', () => {
  const e = new Engine(); // sin resolveTable
  e.setCell('Hoja1', 'A1', '=SUM(Ventas[Ventas])');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#REF!');
});

test('eval: recálculo incremental al editar una celda de la tabla', () => {
  const e = setup();
  e.setCell('Hoja1', 'F1', '=SUM(Ventas[Ventas])');
  expect(e.getValue('Hoja1', 'F1')).toBe(60);
  e.setCell('Hoja1', 'B2', '100'); // 10 → 100, debe propagar por las deps registradas
  expect(e.getValue('Hoja1', 'F1')).toBe(150);
});
