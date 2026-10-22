// Fórmulas de matriz clásicas CSE «{=…}» (Ctrl+Mayús+Entrar): evaluación
// elementwise y derrame al rango. Se ejercita el motor completo. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function setCol(e: Engine, letter: string, vals: string[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, v));
}
function down(e: Engine, a1: string, n: number): CellValue[] {
  const col = a1[0]!;
  const row0 = Number(a1.slice(1));
  return Array.from({ length: n }, (_, i) => e.getValue('Hoja1', `${col}${row0 + i}`));
}

test('CSE: función escalar SQRT se difunde sobre un rango y derrama', () => {
  const e = new Engine();
  setCol(e, 'A', ['4', '9', '16']);
  e.setCell('Hoja1', 'C1', '{=SQRT(A1:A3)}');
  expect(down(e, 'C1', 3)).toEqual([2, 3, 4]);
});

test('CSE: IF elementwise dentro de SUM (patrón SUMIF clásico)', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '5', '2', '8', '3']);
  e.setCell('Hoja1', 'C1', '{=SUM(IF(A1:A5>=5, A1:A5, 0))}');
  expect(e.getValue('Hoja1', 'C1')).toBe(13); // 5 + 8
});

test('CSE: producto de dos rangos sumado (SUMPRODUCT clásico)', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3']);
  setCol(e, 'B', ['4', '5', '6']);
  e.setCell('Hoja1', 'C1', '{=SUM(A1:A3*B1:B3)}');
  expect(e.getValue('Hoja1', 'C1')).toBe(32); // 4+10+18
});

test('CSE: IF con dos ramas de rango, derrama elementwise', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '-2', '3', '-4']);
  e.setCell('Hoja1', 'C1', '{=IF(A1:A4>0, A1:A4, -A1:A4)}'); // valor absoluto
  expect(down(e, 'C1', 4)).toEqual([1, 2, 3, 4]);
});

test('CSE: función de texto LEN se difunde', () => {
  const e = new Engine();
  setCol(e, 'A', ['a', 'bb', 'ccc']);
  e.setCell('Hoja1', 'C1', '{=LEN(A1:A3)}');
  expect(down(e, 'C1', 3)).toEqual([1, 2, 3]);
});

test('CSE: escalar puro {=1+1} devuelve un escalar', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '{=1+1}');
  expect(e.getValue('Hoja1', 'A1')).toBe(2);
});

test('CSE: getRaw conserva las llaves; un literal {x} no es fórmula', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '{=SEQUENCE(2)}');
  expect(e.getRaw('Hoja1', 'A1')).toBe('{=SEQUENCE(2)}');
  e.setCell('Hoja1', 'B1', '{no formula}');
  expect(e.getValue('Hoja1', 'B1')).toBe('{no formula}'); // texto literal
});

test('CSE: choca con celda ocupada → #SPILL!', () => {
  const e = new Engine();
  setCol(e, 'A', ['4', '9']);
  e.setCell('Hoja1', 'C2', '99'); // estorba el derrame
  e.setCell('Hoja1', 'C1', '{=SQRT(A1:A2)}');
  expect(e.getDisplay('Hoja1', 'C1')).toBe('#SPILL!');
});
