// Arrays dinámicos (SEQUENCE/RANDARRAY/UNIQUE/SORT/SORTBY/FILTER), DERRAME
// (spill), difusión de operadores y LET. Se ejercita el motor completo (el
// derrame a celdas contiguas y #SPILL! son responsabilidad del Engine). `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function setCol(e: Engine, letter: string, vals: string[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, v));
}
/** Lee `n` celdas hacia abajo desde `a1` como valores. */
function down(e: Engine, a1: string, n: number): CellValue[] {
  const col = a1[0]!;
  const row0 = Number(a1.slice(1));
  return Array.from({ length: n }, (_, i) => e.getValue('Hoja1', `${col}${row0 + i}`));
}

/* ================= SEQUENCE + derrame ================= */

test('SEQUENCE derrama hacia abajo desde el ancla', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=SEQUENCE(3)');
  expect(down(e, 'A1', 3)).toEqual([1, 2, 3]);
});

test('SEQUENCE lee las celdas derramadas sin haber leído el ancla antes', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=SEQUENCE(3)');
  expect(e.getValue('Hoja1', 'A3')).toBe(3); // A3 se resuelve por el ancla A1
});

test('SEQUENCE 2×3 derrama en bloque (fila mayor)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=SEQUENCE(2, 3)');
  expect(e.getValue('Hoja1', 'A1')).toBe(1);
  expect(e.getValue('Hoja1', 'B1')).toBe(2);
  expect(e.getValue('Hoja1', 'C1')).toBe(3);
  expect(e.getValue('Hoja1', 'A2')).toBe(4);
  expect(e.getValue('Hoja1', 'C2')).toBe(6);
});

test('SEQUENCE con inicio y paso', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=SEQUENCE(3, 1, 10, 5)');
  expect(down(e, 'A1', 3)).toEqual([10, 15, 20]);
});

/* ================= #SPILL! y su liberación ================= */

test('#SPILL! cuando el área de derrame choca; se libera al vaciar', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A2', '99'); // ocupa una celda del futuro derrame
  e.setCell('Hoja1', 'A1', '=SEQUENCE(3)');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#SPILL!');
  expect(e.getValue('Hoja1', 'A2')).toBe(99); // intacta
  e.setCell('Hoja1', 'A2', ''); // libera el estorbo
  expect(down(e, 'A1', 3)).toEqual([1, 2, 3]); // ahora sí derrama
});

test('una celda leída vacía ANTES del derrame se actualiza al cubrirla', () => {
  const e = new Engine();
  expect(e.getValue('Hoja1', 'A3')).toBe(''); // vacía, se lee primero
  e.setCell('Hoja1', 'A1', '=SEQUENCE(3)'); // ahora derrama sobre A2:A3
  expect(e.getValue('Hoja1', 'A3')).toBe(3); // ya no obsoleta
});

test('referenciar una celda derramada la ve; poner contenido allí fuerza #SPILL!', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=SEQUENCE(3)');
  e.setCell('Hoja1', 'B1', '=A2*10');
  expect(e.getValue('Hoja1', 'B1')).toBe(20); // A2 derramada = 2
  e.setCell('Hoja1', 'A2', '7'); // ahora estorba el derrame
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#SPILL!');
});

/* ================= Difusión de operadores sobre arrays ================= */

test('un operador se difunde sobre un rango y derrama', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3']);
  e.setCell('Hoja1', 'C1', '=A1:A3*2');
  expect(down(e, 'C1', 3)).toEqual([2, 4, 6]);
});

/* ================= FILTER ================= */

test('FILTER conserva las filas cuya condición es verdadera', () => {
  const e = new Engine();
  setCol(e, 'A', ['10', '20', '30', '40', '50']);
  e.setCell('Hoja1', 'C1', '=FILTER(A1:A5, A1:A5>=30)');
  expect(down(e, 'C1', 3)).toEqual([30, 40, 50]);
});

test('FILTER sin coincidencias devuelve el «si_vacío» o #CALC!', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3']);
  e.setCell('Hoja1', 'C1', '=FILTER(A1:A3, A1:A3>9, "nada")');
  expect(e.getValue('Hoja1', 'C1')).toBe('nada');
  e.setCell('Hoja1', 'C2', '=FILTER(A1:A3, A1:A3>9)');
  expect(e.getDisplay('Hoja1', 'C2')).toBe('#CALC!');
});

/* ================= SORT / SORTBY / UNIQUE ================= */

test('SORT ascendente y descendente', () => {
  const e = new Engine();
  setCol(e, 'A', ['3', '1', '4', '2']);
  e.setCell('Hoja1', 'C1', '=SORT(A1:A4)');
  expect(down(e, 'C1', 4)).toEqual([1, 2, 3, 4]);
  e.setCell('Hoja1', 'D1', '=SORT(A1:A4, 1, -1)');
  expect(down(e, 'D1', 4)).toEqual([4, 3, 2, 1]);
});

test('SORTBY ordena por un vector paralelo', () => {
  const e = new Engine();
  setCol(e, 'A', ['manzana', 'pera', 'uva']); // valores
  setCol(e, 'B', ['3', '1', '2']); // claves de orden
  e.setCell('Hoja1', 'C1', '=SORTBY(A1:A3, B1:B3)');
  expect(down(e, 'C1', 3)).toEqual(['pera', 'uva', 'manzana']);
});

test('UNIQUE distintos y «solo los que aparecen una vez»', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '2', '3', '3']);
  e.setCell('Hoja1', 'C1', '=UNIQUE(A1:A5)');
  expect(down(e, 'C1', 3)).toEqual([1, 2, 3]);
  e.setCell('Hoja1', 'E1', '=UNIQUE(A1:A5, FALSE, TRUE)'); // exactamente una vez → solo el 1
  expect(e.getValue('Hoja1', 'E1')).toBe(1);
});

/* ================= RANDARRAY (RNG inyectado) ================= */

test('RANDARRAY entero con RNG determinista', () => {
  const e = new Engine({ random: () => 0.5 });
  e.setCell('Hoja1', 'A1', '=RANDARRAY(2, 2, 1, 6, TRUE)'); // floor(0.5*6)+1 = 4
  expect(e.getValue('Hoja1', 'A1')).toBe(4);
  expect(e.getValue('Hoja1', 'B2')).toBe(4);
});

/* ================= LET ================= */

test('LET liga variables locales y las usa en el cálculo', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=LET(x, 5, x + 1)');
  expect(e.getValue('Hoja1', 'A1')).toBe(6);
  e.setCell('Hoja1', 'A2', '=LET(a, 2, b, 3, a * b)'); // varias variables
  expect(e.getValue('Hoja1', 'A2')).toBe(6);
});

test('LET con una variable de rango y derrame', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3', '4']);
  e.setCell('Hoja1', 'C1', '=LET(v, A1:A4, SUM(v))'); // variable = rango entero
  expect(e.getValue('Hoja1', 'C1')).toBe(10);
  e.setCell('Hoja1', 'D1', '=LET(s, SEQUENCE(3), s)'); // devuelve el array → derrama
  expect(down(e, 'D1', 3)).toEqual([1, 2, 3]);
});

test('LET con nombre inválido (referencia) → #VALUE!', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=LET(B1, 5, B1 + 1)'); // B1 es una referencia, no un nombre
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#VALUE!');
});
