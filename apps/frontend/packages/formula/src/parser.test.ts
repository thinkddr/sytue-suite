// Checks del parser: precedencia, rangos, refs absolutas, hoja!celda. `bun test`.
import { expect, test } from 'bun:test';
import { parseFormula, type AstNode } from './parser.js';

function bin(n: AstNode): Extract<AstNode, { kind: 'binary' }> {
  if (n.kind !== 'binary') throw new Error('no es binario: ' + n.kind);
  return n;
}

test('precedencia: * liga más que +', () => {
  const n = bin(parseFormula('=1+2*3'));
  expect(n.op).toBe('+');
  expect(bin(n.right).op).toBe('*');
});

test('ECMA-376: la negación unaria liga más que ^ (-2^2 = 4)', () => {
  const n = parseFormula('=-2^2');
  // Debe ser (-2)^2, no -(2^2).
  expect(n.kind).toBe('binary');
  const b = bin(n);
  expect(b.op).toBe('^');
  expect(b.left.kind).toBe('unary');
});

test('^ es asociativo por la derecha', () => {
  const b = bin(parseFormula('=2^3^2'));
  expect(b.op).toBe('^');
  expect(b.left).toEqual({ kind: 'num', value: 2 });
  expect(bin(b.right).op).toBe('^');
});

test('porcentaje postfijo', () => {
  const n = parseFormula('=50%');
  expect(n.kind).toBe('percent');
});

test('referencia absoluta $A$1', () => {
  const n = parseFormula('=$A$1');
  if (n.kind !== 'ref') throw new Error('esperaba ref');
  expect(n.ref).toMatchObject({ col: 0, row: 0, colAbs: true, rowAbs: true });
});

test('rango A1:B10', () => {
  const n = parseFormula('=A1:B10');
  if (n.kind !== 'range') throw new Error('esperaba range');
  expect(n.a).toMatchObject({ col: 0, row: 0 });
  expect(n.b).toMatchObject({ col: 1, row: 9 });
  expect(n.whole).toBeUndefined(); // retrocompat: rango de celdas normal
});

function range(n: AstNode): Extract<AstNode, { kind: 'range' }> {
  if (n.kind !== 'range') throw new Error('no es range: ' + n.kind);
  return n;
}

test('columna entera A:A y A:C', () => {
  const aa = range(parseFormula('=A:A'));
  expect(aa.whole).toBe('col');
  expect(aa.a).toMatchObject({ col: 0 });
  expect(aa.b).toMatchObject({ col: 0 });
  const ac = range(parseFormula('=A:C'));
  expect(ac.whole).toBe('col');
  expect(ac.a.col).toBe(0);
  expect(ac.b.col).toBe(2);
});

test('columna entera absoluta $A:$B', () => {
  const n = range(parseFormula('=$A:$B'));
  expect(n.whole).toBe('col');
  expect(n.a).toMatchObject({ col: 0, colAbs: true });
  expect(n.b).toMatchObject({ col: 1, colAbs: true });
});

test('fila entera 1:1 y 3:5', () => {
  const one = range(parseFormula('=1:1'));
  expect(one.whole).toBe('row');
  expect(one.a).toMatchObject({ row: 0 });
  expect(one.b).toMatchObject({ row: 0 });
  const n = range(parseFormula('=3:5'));
  expect(n.whole).toBe('row');
  expect(n.a.row).toBe(2);
  expect(n.b.row).toBe(4);
});

test('fila entera absoluta $1:$3', () => {
  const n = range(parseFormula('=$1:$3'));
  expect(n.whole).toBe('row');
  expect(n.a).toMatchObject({ row: 0, rowAbs: true });
  expect(n.b).toMatchObject({ row: 2, rowAbs: true });
});

test('columna/fila entera con hoja: Hoja1!A:A y Datos!3:5', () => {
  const col = range(parseFormula('=Hoja1!A:A'));
  expect(col.whole).toBe('col');
  expect(col.a.sheet).toBe('Hoja1');
  const row = range(parseFormula('=Datos!3:5'));
  expect(row.whole).toBe('row');
  expect(row.a.sheet).toBe('Datos');
  expect(row.a.row).toBe(2);
  expect(row.b.row).toBe(4);
});

test('extremos invertidos se normalizan (C:A → A:C, 5:3 → 3:5)', () => {
  const col = range(parseFormula('=C:A'));
  expect(col.a.col).toBe(0);
  expect(col.b.col).toBe(2);
  const row = range(parseFormula('=5:3'));
  expect(row.a.row).toBe(2);
  expect(row.b.row).toBe(4);
});

test('extremos de rango incompatibles lanzan (A1:B, A:3)', () => {
  expect(() => parseFormula('=A1:B')).toThrow();
  expect(() => parseFormula('=A:3')).toThrow();
});

test("referencia con hoja: Hoja1!A1 y 'Mi Hoja'!B2", () => {
  const a = parseFormula('=Hoja1!A1');
  if (a.kind !== 'ref') throw new Error('ref');
  expect(a.ref.sheet).toBe('Hoja1');
  const b = parseFormula("='Mi Hoja'!B2");
  if (b.kind !== 'ref') throw new Error('ref');
  expect(b.ref.sheet).toBe('Mi Hoja');
  expect(b.ref).toMatchObject({ col: 1, row: 1 });
});

test('llamada con «;» y con «,» como separador', () => {
  const a = parseFormula('=SUM(1;2;3)');
  const b = parseFormula('=SUM(1,2,3)');
  if (a.kind !== 'call' || b.kind !== 'call') throw new Error('call');
  expect(a.args.length).toBe(3);
  expect(b.args.length).toBe(3);
  expect(a.name).toBe('SUM');
});

test('concatenación y comparadores', () => {
  expect(bin(parseFormula('="a"&"b"')).op).toBe('&');
  expect(bin(parseFormula('=A1<>B1')).op).toBe('<>');
  expect(bin(parseFormula('=A1<=B1')).op).toBe('<=');
});
