// Texto moderno: TEXTSPLIT/TEXTBEFORE/TEXTAFTER/ARRAYTOTEXT + HYPERLINK.
// Contrastado con una hoja real. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function calc(formula: string, cells: Record<string, string> = {}): CellValue {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}

test('TEXTSPLIT en columnas (derrama a la derecha)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=TEXTSPLIT("uno-dos-tres", "-")');
  expect(e.getValue('Hoja1', 'A1')).toBe('uno');
  expect(e.getValue('Hoja1', 'B1')).toBe('dos');
  expect(e.getValue('Hoja1', 'C1')).toBe('tres');
});

test('TEXTSPLIT con delimitador de fila y de columna (matriz)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=TEXTSPLIT("a,b;c,d", ",", ";")');
  expect(e.getValue('Hoja1', 'A1')).toBe('a');
  expect(e.getValue('Hoja1', 'B1')).toBe('b');
  expect(e.getValue('Hoja1', 'A2')).toBe('c');
  expect(e.getValue('Hoja1', 'B2')).toBe('d');
});

test('TEXTSPLIT ignora vacíos y rellena filas irregulares con #N/A', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=TEXTSPLIT("a,,b", ",", "", TRUE)'); // ignora el token vacío
  expect(e.getValue('Hoja1', 'A1')).toBe('a');
  expect(e.getValue('Hoja1', 'B1')).toBe('b');
  e.setCell('Hoja1', 'A3', '=TEXTSPLIT("a,b;c", ",", ";")'); // 2ª fila más corta
  expect(e.getValue('Hoja1', 'A4')).toBe('c');
  expect(e.getDisplay('Hoja1', 'B4')).toBe('#N/A'); // relleno
});

test('TEXTBEFORE / TEXTAFTER con instancia (positiva y negativa)', () => {
  expect(calc('=TEXTBEFORE("a-b-c", "-")')).toBe('a');
  expect(calc('=TEXTBEFORE("a-b-c", "-", 2)')).toBe('a-b');
  expect(calc('=TEXTAFTER("a-b-c", "-")')).toBe('b-c');
  expect(calc('=TEXTAFTER("a-b-c", "-", 2)')).toBe('c');
  expect(calc('=TEXTBEFORE("a-b-c", "-", -1)')).toBe('a-b'); // última aparición
  expect(calc('=TEXTAFTER("a-b-c", "-", -1)')).toBe('c');
});

test('TEXTBEFORE: no encontrado → #N/A o «si_no_hay»', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', '=TEXTBEFORE("abc", "x")');
  expect(e.getDisplay('Hoja1', 'Z1')).toBe('#N/A');
  e.setCell('Hoja1', 'Z2', '=TEXTBEFORE("abc", "x", 1, 0, 0, "no")');
  expect(e.getValue('Hoja1', 'Z2')).toBe('no');
});

test('ARRAYTOTEXT conciso y estricto', () => {
  expect(calc('=ARRAYTOTEXT(A1:C1)', { A1: '1', B1: 'a', C1: 'TRUE' })).toBe('1, a, TRUE');
  expect(calc('=ARRAYTOTEXT(A1:C1, 1)', { A1: '1', B1: 'a', C1: 'TRUE' })).toBe('{1,"a",TRUE}');
  expect(calc('=ARRAYTOTEXT(A1:B2, 1)', { A1: '1', B1: '2', A2: '3', B2: '4' })).toBe('{1,2;3,4}');
});

test('HYPERLINK devuelve el nombre visible (o el destino)', () => {
  expect(calc('=HYPERLINK("http://sytue.com", "Sytue")')).toBe('Sytue');
  expect(calc('=HYPERLINK("http://sytue.com")')).toBe('http://sytue.com');
});
