// Checks del motor: recálculo, dependencias, ciclo→#CIRC!, SUM/IF. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from './engine.js';
import type { FormulaError } from './values.js';

test('literales: número, texto, booleano', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '42');
  e.setCell('Hoja1', 'A2', 'hola');
  e.setCell('Hoja1', 'A3', 'TRUE');
  expect(e.getValue('Hoja1', 'A1')).toBe(42);
  expect(e.getValue('Hoja1', 'A2')).toBe('hola');
  expect(e.getValue('Hoja1', 'A3')).toBe(true);
});

test('aritmética y precedencia coincide con el display', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=1+2*3');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('7');
  e.setCell('Hoja1', 'A2', '=-2^2'); // ECMA-376 → 4
  expect(e.getValue('Hoja1', 'A2')).toBe(4);
});

test('coma flotante formateada (0.1+0.2 → 0.3)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=0.1+0.2');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('0.3');
});

test('#DIV/0! al dividir por cero', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=1/0');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#DIV/0!');
});

test('dependencias y recálculo incremental', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '10');
  e.setCell('Hoja1', 'A2', '=A1*2');
  e.setCell('Hoja1', 'A3', '=A2+1');
  expect(e.getValue('Hoja1', 'A3')).toBe(21);
  e.setCell('Hoja1', 'A1', '5'); // debe propagar
  expect(e.getValue('Hoja1', 'A2')).toBe(10);
  expect(e.getValue('Hoja1', 'A3')).toBe(11);
});

test('ciclo → #CIRC!', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=B1');
  e.setCell('Hoja1', 'B1', '=A1');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#CIRC!');
  expect(e.getDisplay('Hoja1', 'B1')).toBe('#CIRC!');
});

test('auto-referencia → #CIRC!', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=A1+1');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#CIRC!');
});

test('SUM sobre rango y escalares, propaga error', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '1');
  e.setCell('Hoja1', 'A2', '2');
  e.setCell('Hoja1', 'A3', '3');
  e.setCell('Hoja1', 'A4', 'texto'); // se ignora
  e.setCell('Hoja1', 'B1', '=SUM(A1:A4)');
  expect(e.getValue('Hoja1', 'B1')).toBe(6);
  e.setCell('Hoja1', 'B2', '=SUM(A1:A3,10,TRUE)'); // bool escalar cuenta
  expect(e.getValue('Hoja1', 'B2')).toBe(17);
  e.setCell('Hoja1', 'A2', '=1/0');
  expect(e.getDisplay('Hoja1', 'B1')).toBe('#DIV/0!');
});

test('columna entera A:A: SUM/COUNT acotados al rango usado', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '1');
  e.setCell('Hoja1', 'A2', '2');
  e.setCell('Hoja1', 'A3', '3');
  e.setCell('Hoja1', 'A5', 'texto'); // se ignora en SUM; cuenta en COUNTA, no en COUNT
  e.setCell('Hoja1', 'B1', '=SUM(A:A)');
  e.setCell('Hoja1', 'B2', '=COUNT(A:A)');
  expect(e.getValue('Hoja1', 'B1')).toBe(6);
  expect(e.getValue('Hoja1', 'B2')).toBe(3);
});

test('columna entera: recalcula al añadir contenido MÁS ABAJO del usado', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '1');
  e.setCell('Hoja1', 'A2', '2');
  e.setCell('Hoja1', 'B1', '=SUM(A:A)');
  expect(e.getValue('Hoja1', 'B1')).toBe(3);
  e.setCell('Hoja1', 'A9', '10'); // fila nueva por debajo del máximo previo
  expect(e.getValue('Hoja1', 'B1')).toBe(13);
  e.setCell('Hoja1', 'A9', ''); // borrar también recalcula
  expect(e.getValue('Hoja1', 'B1')).toBe(3);
});

test('columna entera vacía → SUM 0; varias columnas A:B', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '10');
  e.setCell('Hoja1', 'B1', '5');
  e.setCell('Hoja1', 'B2', '5');
  e.setCell('Hoja1', 'D1', '=SUM(Z:Z)'); // columna sin nada
  e.setCell('Hoja1', 'D2', '=SUM(A:B)');
  expect(e.getValue('Hoja1', 'D1')).toBe(0);
  expect(e.getValue('Hoja1', 'D2')).toBe(20);
});

test('fila entera 1:1 / 3:5 acotada a las columnas usadas', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '10');
  e.setCell('Hoja1', 'B1', '20');
  e.setCell('Hoja1', 'C1', '30');
  e.setCell('Hoja1', 'A3', '1');
  e.setCell('Hoja1', 'A5', '4');
  // Las fórmulas van FUERA de las filas sumadas (una fórmula dentro de su propia
  // fila entera sería circular, como en Excel).
  e.setCell('Hoja1', 'E10', '=SUM(1:1)');
  e.setCell('Hoja1', 'E11', '=SUM(3:5)');
  expect(e.getValue('Hoja1', 'E10')).toBe(60);
  expect(e.getValue('Hoja1', 'E11')).toBe(5);
});

test('extremos invertidos (C:A) evalúan igual que A:C', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '1');
  e.setCell('Hoja1', 'B1', '2');
  e.setCell('Hoja1', 'C1', '3');
  e.setCell('Hoja1', 'E1', '=SUM(C:A)');
  expect(e.getValue('Hoja1', 'E1')).toBe(6);
});

test('columna entera que se incluye a sí misma → #CIRC!', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '1');
  e.setCell('Hoja1', 'A2', '=SUM(A:A)'); // A2 está en la columna A → circular
  expect(e.getDisplay('Hoja1', 'A2')).toBe('#CIRC!');
});

test('columna entera con hoja: Hoja1!A:A cruzada', () => {
  const e = new Engine({ sheets: ['Hoja1', 'Datos'] });
  e.setCell('Datos', 'A1', '5');
  e.setCell('Datos', 'A2', '7');
  e.setCell('Hoja1', 'B1', '=SUM(Datos!A:A)');
  expect(e.getValue('Hoja1', 'B1')).toBe(12);
  e.setCell('Datos', 'A3', '3'); // invalida a través de la hoja
  expect(e.getValue('Hoja1', 'B1')).toBe(15);
});

test('IF perezoso: no evalúa la rama no tomada', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=IF(TRUE, 1, 1/0)');
  expect(e.getValue('Hoja1', 'A1')).toBe(1); // la rama 1/0 no se evalúa
  e.setCell('Hoja1', 'A2', '=IF(1>2, "sí", "no")');
  expect(e.getValue('Hoja1', 'A2')).toBe('no');
});

test('referencias entre hojas', () => {
  const e = new Engine({ sheets: ['Hoja1', 'Datos'] });
  e.setCell('Datos', 'A1', '100');
  e.setCell('Hoja1', 'A1', '=Datos!A1+1');
  expect(e.getValue('Hoja1', 'A1')).toBe(101);
});

test('#NAME? en función desconocida', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=NOEXISTE(1)');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#NAME?');
});

test('recalc() no rompe y refleja cambios', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=SUM(1,2)');
  e.recalc();
  expect(e.getValue('Hoja1', 'A1')).toBe(3);
});

test('inyección de reloj/RNG deterministas', () => {
  const fixed = new Date('2026-07-04T00:00:00Z');
  const e = new Engine({ now: () => fixed, random: () => 0.5 });
  // No hay funciones que los usen aún, pero el ctx debe exponerlos sin fallar.
  e.setCell('Hoja1', 'A1', '=1');
  expect(e.getValue('Hoja1', 'A1')).toBe(1);
});

test('usedRange: la última columna y fila con contenido; -1 en una hoja vacía', () => {
  const e = new Engine();
  expect(e.usedRange('Hoja1')).toEqual({ maxCol: -1, maxRow: -1 });
  e.setCell('Hoja1', 'B3', '1');
  e.setCell('Hoja1', 'AD5000', 'x'); // más allá de A..Z × 1..100
  expect(e.usedRange('Hoja1')).toEqual({ maxCol: 29, maxRow: 4999 });
});

test('evaluate: calcula sin ocupar celda ni estirar el rango usado (ni los A:A)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '5');
  e.setCell('Hoja1', 'A2', '7');
  e.setCell('Hoja1', 'B1', '=SUM(A:A)');
  expect(e.evaluate('Hoja1', '=A1>3')).toBe(true);
  expect(e.evaluate('Hoja1', '=ROW()', 0, 41)).toBe(42); // se evalúa «como si» estuviera ahí
  expect((e.evaluate('Hoja1', '=(') as FormulaError).code).toBe('#NAME?'); // no parsea
  // La celda de apoyo de antes (ZZ1000) estiraba lo usado y con ello cada A:A.
  expect(e.usedRange('Hoja1')).toEqual({ maxCol: 1, maxRow: 1 });
  expect(e.getValue('Hoja1', 'B1')).toBe(12);
});
