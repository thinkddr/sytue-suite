// REGEX* (máquina lineal de regex.ts) y fechas nuevas (ISOWEEKNUM/YEARFRAC/DATESTR).
// `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function calc(formula: string): CellValue {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}
function show(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('REGEXTEST', () => {
  expect(calc('=REGEXTEST("hello123", "[0-9]+")')).toBe(true);
  expect(calc('=REGEXTEST("hello", "[0-9]+")')).toBe(false);
  expect(calc('=REGEXTEST("HELLO", "hello", 1)')).toBe(true); // insensible a caja
  expect(show('=REGEXTEST("x", "[")')).toBe('#VALUE!'); // patrón inválido
});

test('REGEXEXTRACT modo 0 (primera coincidencia)', () => {
  expect(calc('=REGEXEXTRACT("id=42;x=7", "[0-9]+")')).toBe('42');
  expect(show('=REGEXEXTRACT("abc", "[0-9]+")')).toBe('#N/A');
});

test('REGEXEXTRACT modo 1 (todas, derrama en columna)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=REGEXEXTRACT("a1b2c3", "[0-9]", 1)');
  expect(e.getValue('Hoja1', 'A1')).toBe('1');
  expect(e.getValue('Hoja1', 'A2')).toBe('2');
  expect(e.getValue('Hoja1', 'A3')).toBe('3');
});

test('REGEXEXTRACT modo 2 (grupos de captura, derrama en fila)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=REGEXEXTRACT("2024-01-15", "(\\d+)-(\\d+)-(\\d+)", 2)');
  expect(e.getValue('Hoja1', 'A1')).toBe('2024');
  expect(e.getValue('Hoja1', 'B1')).toBe('01');
  expect(e.getValue('Hoja1', 'C1')).toBe('15');
});

test('REGEXREPLACE (todas, n-ésima y backrefs)', () => {
  expect(calc('=REGEXREPLACE("a1b2", "[0-9]", "#")')).toBe('a#b#');
  expect(calc('=REGEXREPLACE("a1b2c3", "[0-9]", "#", 2)')).toBe('a1b#c3'); // solo la 2ª
  expect(calc('=REGEXREPLACE("John Smith", "(\\w+) (\\w+)", "$2 $1")')).toBe('Smith John');
});

// WEB-13. Las REGEX* corren en una máquina lineal (regex.ts): un patrón de vuelta atrás
// catastrófica ya no congela la pestaña de quien abre la hoja. Con el RegExp de
// JavaScript, «(a+)+$» sobre 28 aes y un «!» tardaba segundos, y con 40, para siempre.
test('REGEX* en tiempo lineal: la vuelta atrás catastrófica no congela', () => {
  const t0 = performance.now();
  expect(calc(`=REGEXTEST("${'a'.repeat(28)}!", "(a+)+$")`)).toBe(false);
  expect(calc(`=REGEXTEST("${'a'.repeat(5000)}!", "^(a|aa)*$")`)).toBe(false);
  expect(calc(`=REGEXTEST("${'a'.repeat(5000)}", "^(a|aa)*$")`)).toBe(true);
  expect(performance.now() - t0).toBeLessThan(500);
});

test('REGEX* con presupuesto: lo cuadrático desmesurado es #VALUE!, no una pestaña colgada', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'B1', 'a'.repeat(30000));
  const t0 = performance.now();
  e.setCell('Hoja1', 'A1', '=REGEXEXTRACT(B1, "a(?:.*z)?", 1)');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#VALUE!');
  expect(performance.now() - t0).toBeLessThan(5000);
});

test('REGEX* como RE2 (el motor nativo): sin referencias hacia atrás ni lookaround', () => {
  expect(show('=REGEXTEST("aa", "(a)\\1")')).toBe('#VALUE!');
  expect(show('=REGEXTEST("ab", "a(?=b)")')).toBe('#VALUE!');
  expect(show('=REGEXTEST("ab", "(?<!a)b")')).toBe('#VALUE!');
  // La más a la izquierda y, ahí, la que prefieren las alternativas y los cuantificadores.
  expect(calc('=REGEXEXTRACT("abc", "a|ab")')).toBe('a');
  expect(calc('=REGEXEXTRACT("<a><b>", "<.+?>")')).toBe('<a>');
  expect(calc('=REGEXEXTRACT("<a><b>", "<.+>")')).toBe('<a><b>');
  expect(calc('=REGEXEXTRACT("x2024-01-15", "(?<a>\\d{4})-(\\d{2})")')).toBe('2024-01');
  expect(calc('=REGEXTEST("ABC", "^[a-c]+$", 1)')).toBe(true);
  expect(calc('=REGEXTEST("ABC", "^[a-c]+$")')).toBe(false);
  expect(calc('=REGEXTEST("fin de linea", "\\bde\\b")')).toBe(true);
  // Una vacía pegada a la anterior no cuenta, como FindAll de Go.
  expect(calc('=REGEXREPLACE("aXb", "X*", "-")')).toBe('-a-b-');
  expect(calc('=REGEXREPLACE("ab", "(a)|(b)", "[$1$2]")')).toBe('[a][b]');
  expect(calc('=REGEXREPLACE("ab", "b", "$`$\'$$")')).toBe('aa$');
});

test('ISOWEEKNUM', () => {
  expect(calc('=ISOWEEKNUM(DATE(2016,1,1))')).toBe(53); // viernes → semana 53 de 2015
  expect(calc('=ISOWEEKNUM(DATE(2024,1,1))')).toBe(1); // lunes
  expect(calc('=ISOWEEKNUM(DATE(2024,12,31))')).toBe(1); // martes → semana 1 de 2025
});

test('YEARFRAC por base', () => {
  expect(calc('=YEARFRAC(DATE(2024,1,1), DATE(2024,7,1), 0)')).toBeCloseTo(0.5, 9); // 30/360
  expect(calc('=YEARFRAC(DATE(2024,1,1), DATE(2024,7,1), 4)')).toBeCloseTo(0.5, 9); // 30E/360
  expect(calc('=YEARFRAC(DATE(2023,1,1), DATE(2023,7,1), 3)')).toBeCloseTo(181 / 365, 9); // act/365
  expect(calc('=YEARFRAC(DATE(2024,1,1), DATE(2024,7,1), 2)')).toBeCloseTo(182 / 360, 9); // act/360
  expect(calc('=YEARFRAC(DATE(2024,1,1), DATE(2024,12,31), 1)')).toBeCloseTo(365 / 366, 9); // act/act
  // simétrica: orden invertido da el mismo valor positivo
  expect(calc('=YEARFRAC(DATE(2024,7,1), DATE(2024,1,1), 0)')).toBeCloseTo(0.5, 9);
});

test('DATESTR (inverso de DATEVALUE)', () => {
  expect(calc('=DATESTR(DATE(2024,3,7))')).toBe('2024-03-07');
  expect(calc('=DATEVALUE(DATESTR(DATE(2024,3,7)))')).toBe(calc('=DATE(2024,3,7)'));
});
