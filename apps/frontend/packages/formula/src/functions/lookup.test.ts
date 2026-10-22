// Checks de búsqueda/referencia: valores conocidos de una hoja real, casos de
// error, refs y formas de matriz. `bun test`. La mayoría van end-to-end por el
// Engine (resuelve refs/rangos de verdad); las que devuelven MATRIZ (que el
// Engine colapsaría a #VALUE! en una celda escalar) se prueban llamando a
// `funcs` directamente con Args de mentira.
import { expect, test } from 'bun:test';
import { funcs } from './lookup.js';
import { Engine } from '../engine.js';
import { numToCol } from '../parser.js';
import { ERR, isError, type CellValue } from '../values.js';
import type { Arg, FnCtx } from '../registry.js';

/** Hoja de trabajo compartida: tabla de datos para todas las búsquedas. */
function grid(): Engine {
  const e = new Engine();
  [1, 2, 3, 4].forEach((n, i) => e.setCell('Hoja1', `A${i + 1}`, String(n))); // A: números asc
  ['uno', 'dos', 'tres', 'cuatro'].forEach((s, i) => e.setCell('Hoja1', `B${i + 1}`, s));
  ['Manzana', 'Pera', 'Uva', 'Kiwi'].forEach((s, i) => e.setCell('Hoja1', `C${i + 1}`, s));
  [10, 20, 30, 40].forEach((n, i) => e.setCell('Hoja1', `D${i + 1}`, String(n)));
  [4, 3, 2, 1].forEach((n, i) => e.setCell('Hoja1', `E${i + 1}`, String(n))); // E: números desc
  [1, 2, 3, 4].forEach((n, i) => e.setCell('Hoja1', `${numToCol(i)}6`, String(n))); // fila 6 asc
  ['a', 'b', 'c', 'd'].forEach((s, i) => e.setCell('Hoja1', `${numToCol(i)}7`, s)); // fila 7
  return e;
}

/** Evalúa una fórmula en una celda de sobra (Z1) y devuelve el valor. */
function cv(e: Engine, formula: string): CellValue {
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}
/** Igual que cv() pero el texto a mostrar (para códigos de error/formato). */
function cd(e: Engine, formula: string): string {
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

/* ================= VLOOKUP / HLOOKUP ================= */

test('VLOOKUP: exacta, aproximada y errores', () => {
  const e = grid();
  expect(cv(e, '=VLOOKUP(3, A1:B4, 2, FALSE)')).toBe('tres');
  expect(cv(e, '=VLOOKUP(3, A1:B4, 2)')).toBe('tres'); // aprox con exacta presente
  expect(cv(e, '=VLOOKUP(2.5, A1:B4, 2)')).toBe('dos'); // mayor ≤ 2.5 → fila 2
  expect(cv(e, '=VLOOKUP("uva", C1:D4, 2, FALSE)')).toBe(30); // texto, sin distinguir caja
  expect(cd(e, '=VLOOKUP(0, A1:B4, 2)')).toBe('#N/A'); // por debajo del menor
  expect(cd(e, '=VLOOKUP(9, A1:B4, 2, FALSE)')).toBe('#N/A'); // exacta no hallada
  expect(cd(e, '=VLOOKUP(3, A1:B4, 3)')).toBe('#REF!'); // col fuera de rango
  expect(cd(e, '=VLOOKUP(3, A1:B4, 0)')).toBe('#VALUE!'); // col < 1
  expect(cd(e, '=VLOOKUP(1/0, A1:B4, 2)')).toBe('#DIV/0!'); // error en el valor buscado
});

test('HLOOKUP: busca en la primera fila', () => {
  const e = grid();
  expect(cv(e, '=HLOOKUP(3, A6:D7, 2, FALSE)')).toBe('c');
  expect(cv(e, '=HLOOKUP(2.5, A6:D7, 2)')).toBe('b'); // aprox
  expect(cd(e, '=HLOOKUP(3, A6:D7, 3)')).toBe('#REF!'); // sólo 2 filas
});

/* ================= XLOOKUP / XMATCH ================= */

test('XLOOKUP: exacta, si_no_encontrado y modos ±1', () => {
  const e = grid();
  expect(cv(e, '=XLOOKUP(3, A1:A4, B1:B4)')).toBe('tres');
  expect(cv(e, '=XLOOKUP(9, A1:A4, B1:B4, "no")')).toBe('no'); // reserva
  expect(cd(e, '=XLOOKUP(9, A1:A4, B1:B4)')).toBe('#N/A'); // sin reserva
  expect(cv(e, '=XLOOKUP(2.5, A1:A4, B1:B4, "no", -1)')).toBe('dos'); // siguiente menor
  expect(cv(e, '=XLOOKUP(2.5, A1:A4, B1:B4, "no", 1)')).toBe('tres'); // siguiente mayor
});

test('XLOOKUP: devuelve la fila entera del bloque (matriz)', () => {
  // la vertical (3 filas), ra bloque 3×2 → la coincidencia devuelve su fila.
  const la = rng([[1], [2], [3]]);
  const ra = rng([
    [10, 100],
    [20, 200],
    [30, 300],
  ]);
  expect(call('XLOOKUP', [arg(2), la, ra])).toEqual([[20, 200]]);
});

test('XMATCH: posición con modos de coincidencia', () => {
  const e = grid();
  expect(cv(e, '=XMATCH(3, A1:A4)')).toBe(3);
  expect(cv(e, '=XMATCH(2.5, A1:A4, -1)')).toBe(2); // siguiente menor
  expect(cv(e, '=XMATCH(2.5, A1:A4, 1)')).toBe(3); // siguiente mayor
  expect(cd(e, '=XMATCH(9, A1:A4)')).toBe('#N/A');
});

/* ================= INDEX / MATCH / LOOKUP ================= */

test('INDEX: celda, vector y errores', () => {
  const e = grid();
  expect(cv(e, '=INDEX(B1:B4, 2)')).toBe('dos'); // columna única → índice de fila
  expect(cv(e, '=INDEX(A6:D6, 3)')).toBe(3); // fila única → índice de columna
  expect(cv(e, '=INDEX(C1:D4, 3, 2)')).toBe(30); // 2D
  expect(cd(e, '=INDEX(C1:D4, 5, 1)')).toBe('#REF!'); // fila fuera de rango
  expect(cd(e, '=INDEX(C1:D4, -1, 1)')).toBe('#VALUE!'); // índice negativo
  expect(cv(e, '=INDEX(A1:A4, MATCH(3, A1:A4, 0))')).toBe(3); // combo clásico INDEX/MATCH
});

test('INDEX: fila/columna 0 devuelven la fila/columna entera (matriz)', () => {
  const m = rng([
    [1, 2],
    [3, 4],
    [5, 6],
  ]);
  expect(call('INDEX', [m, arg(0), arg(2)])).toEqual([[2], [4], [6]]); // columna 2 entera
  expect(call('INDEX', [m, arg(2), arg(0)])).toEqual([[3, 4]]); // fila 2 entera
});

test('MATCH: exacta y aproximada asc/desc', () => {
  const e = grid();
  expect(cv(e, '=MATCH(3, A1:A4, 0)')).toBe(3);
  expect(cv(e, '=MATCH(2.5, A1:A4, 1)')).toBe(2); // mayor ≤ (asc)
  expect(cv(e, '=MATCH(2.5, A1:A4)')).toBe(2); // tipo por defecto 1
  expect(cv(e, '=MATCH("Uva", C1:C4, 0)')).toBe(3); // texto sin distinguir caja
  expect(cv(e, '=MATCH(2.5, E1:E4, -1)')).toBe(2); // menor ≥ (desc): E=[4,3,2,1]
  expect(cd(e, '=MATCH(9, A1:A4, 0)')).toBe('#N/A');
});

test('LOOKUP: forma vectorial y de matriz', () => {
  const e = grid();
  expect(cv(e, '=LOOKUP(2.5, A1:A4, B1:B4)')).toBe('dos'); // vectorial aprox
  expect(cv(e, '=LOOKUP(3, A1:B4)')).toBe('tres'); // matriz alta: 1ª col → última col
  expect(cv(e, '=LOOKUP(3, A6:D7)')).toBe('c'); // matriz ancha: 1ª fila → última fila
  expect(cd(e, '=LOOKUP(0, A1:A4, B1:B4)')).toBe('#N/A');
});

/* ================= Comodines en coincidencia exacta ================= */

test('comodines: VLOOKUP/HLOOKUP FALSE, MATCH 0, XLOOKUP/XMATCH modo 2', () => {
  const e = grid();
  expect(cv(e, '=VLOOKUP("Man*", C1:D4, 2, FALSE)')).toBe(10); // «*» tras Man → Manzana
  expect(cv(e, '=VLOOKUP("Ki?i", C1:D4, 2, FALSE)')).toBe(40); // «?» un carácter → Kiwi
  expect(cv(e, '=MATCH("*va", C1:C4, 0)')).toBe(3); // termina en «va» → Uva
  expect(cv(e, '=HLOOKUP("b", A7:D7, 1, FALSE)')).toBe('b'); // sin comodín → exacta normal
  expect(cv(e, '=XLOOKUP("Pe*", C1:C4, D1:D4, "no", 2)')).toBe(20); // modo 2 = comodín
  expect(cv(e, '=XMATCH("*wi", C1:C4, 2)')).toBe(4); // Kiwi
  expect(cd(e, '=XLOOKUP("*va", C1:C4, D1:D4, "no", 0)')).toBe('no'); // modo 0 NO usa comodines
  expect(cd(e, '=VLOOKUP("Man~*", C1:D4, 2, FALSE)')).toBe('#N/A'); // «~*» = «*» literal, no existe
});

/* ================= CHOOSE ================= */

test('CHOOSE: selecciona, es perezoso y admite rangos', () => {
  const e = grid();
  expect(cv(e, '=CHOOSE(2, "a", "b", "c")')).toBe('b');
  expect(cv(e, '=CHOOSE(1, 10, 20)')).toBe(10);
  expect(cd(e, '=CHOOSE(3, "a", "b")')).toBe('#VALUE!'); // fuera de rango
  expect(cv(e, '=CHOOSE(1, 5, 1/0)')).toBe(5); // no evalúa la rama 2 (perezoso)
  expect(cv(e, '=SUM(CHOOSE(2, A1:A4, D1:D4))')).toBe(100); // devuelve el rango entero → SUM lo consume
  expect(cv(e, '=SUM(CHOOSE(1, A1:A4, D1:D4))')).toBe(10);
});

/* ================= OFFSET / INDIRECT ================= */

test('OFFSET: desplaza y redimensiona', () => {
  const e = grid();
  expect(cv(e, '=OFFSET(A1, 2, 0)')).toBe(3); // → A3
  expect(cv(e, '=OFFSET(A1, 0, 1)')).toBe('uno'); // → B1
  expect(cv(e, '=SUM(OFFSET(A1, 0, 0, 4, 1))')).toBe(10); // A1:A4
  expect(cd(e, '=OFFSET(A1, -1, 0)')).toBe('#REF!'); // fuera del grid
  expect(cd(e, '=OFFSET(A1, 0, 0, 0, 1)')).toBe('#REF!'); // alto 0
  expect(cd(e, '=OFFSET(5, 1, 1)')).toBe('#VALUE!'); // el 1er arg no es referencia
});

test('INDIRECT: interpreta texto como referencia', () => {
  const e = grid();
  expect(cv(e, '=INDIRECT("A3")')).toBe(3);
  expect(cv(e, '=INDIRECT("B" & 2)')).toBe('dos'); // dirección construida
  expect(cv(e, '=SUM(INDIRECT("A1:A4"))')).toBe(10);
  expect(cd(e, '=INDIRECT("Z")')).toBe('#REF!'); // no es una referencia
  expect(cd(e, '=INDIRECT("A1", FALSE)')).toBe('#REF!'); // R1C1 no soportado
});

test('INDIRECT: ve el valor actual de la celda destino', () => {
  const e = grid();
  e.setCell('Hoja1', 'A2', '99'); // cambia el destino
  expect(cv(e, '=INDIRECT("A2")')).toBe(99);
});

/* ================= ROW / COLUMN / ROWS / COLUMNS ================= */

test('ROW/COLUMN: celda actual y de una referencia', () => {
  const e = grid();
  e.setCell('Hoja1', 'A10', '=ROW()');
  e.setCell('Hoja1', 'C10', '=COLUMN()');
  expect(e.getValue('Hoja1', 'A10')).toBe(10);
  expect(e.getValue('Hoja1', 'C10')).toBe(3);
  expect(cv(e, '=ROW(D5)')).toBe(5);
  expect(cv(e, '=COLUMN(D5)')).toBe(4);
});

test('ROW/COLUMN de un rango devuelven un vector', () => {
  expect(call('ROW', [rng([[1], [2], [3]])])).toEqual([[1], [2], [3]]); // ref filas 0..2
  expect(call('COLUMN', [rng([[1, 2, 3]])])).toEqual([[1, 2, 3]]); // ref cols 0..2
  expect(call('ROW', [])).toBe(10); // sin arg → here.row(9)+1
  expect(call('COLUMN', [])).toBe(3); // sin arg → here.col(2)+1
});

test('ROWS/COLUMNS: dimensiones', () => {
  const e = grid();
  expect(cv(e, '=ROWS(A1:A4)')).toBe(4);
  expect(cv(e, '=ROWS(C1:D4)')).toBe(4);
  expect(cv(e, '=COLUMNS(A6:D6)')).toBe(4);
  expect(cv(e, '=COLUMNS(C1:D4)')).toBe(2);
});

/* ================= TRANSPOSE ================= */

test('TRANSPOSE: intercambia filas y columnas', () => {
  expect(call('TRANSPOSE', [rng([[1, 2, 3]])])).toEqual([[1], [2], [3]]);
  expect(
    call('TRANSPOSE', [
      rng([
        [1, 2],
        [3, 4],
      ]),
    ]),
  ).toEqual([
    [1, 3],
    [2, 4],
  ]);
  const e = grid();
  expect(cv(e, '=SUM(TRANSPOSE(A1:A4))')).toBe(10); // no cambia la suma
});

/* ================= ADDRESS ================= */

test('ADDRESS: estilos absoluto/relativo, R1C1 y hoja', () => {
  const e = grid();
  expect(cv(e, '=ADDRESS(1, 1)')).toBe('$A$1');
  expect(cv(e, '=ADDRESS(2, 3)')).toBe('$C$2');
  expect(cv(e, '=ADDRESS(2, 3, 2)')).toBe('C$2');
  expect(cv(e, '=ADDRESS(2, 3, 3)')).toBe('$C2');
  expect(cv(e, '=ADDRESS(2, 3, 4)')).toBe('C2');
  expect(cv(e, '=ADDRESS(1, 1, 1, FALSE)')).toBe('R1C1');
  expect(cv(e, '=ADDRESS(2, 3, 4, FALSE)')).toBe('R[2]C[3]');
  expect(cv(e, '=ADDRESS(2, 3, 1, TRUE, "Hoja1")')).toBe('Hoja1!$C$2');
  expect(cv(e, '=ADDRESS(2, 3, 1, TRUE, "Mi Hoja")')).toBe("'Mi Hoja'!$C$2"); // se cita
  expect(cd(e, '=ADDRESS(0, 1)')).toBe('#VALUE!'); // fila < 1
});

/* ================= Utilidades para pruebas directas de matriz ================= */

const ctx: FnCtx = {
  now: () => new Date(0),
  random: () => 0,
  here: { sheet: 'Hoja1', col: 2, row: 9 }, // ROW()=10, COLUMN()=3
  resolve: () => [],
  parseRef: () => undefined,
  isFormula: () => false,
  resolveTable: () => undefined,
};

/** Arg escalar (sin referencia). */
function arg(v: CellValue): Arg {
  return { value: () => v, matrix: () => [[v]], ref: undefined };
}

/** Arg de rango: matriz + `ref` 0-based que abarca toda la matriz. */
function rng(m: CellValue[][]): Arg {
  return {
    value: () => (m.length === 1 && m[0]!.length === 1 ? m[0]![0]! : ERR.VALUE),
    matrix: () => m,
    ref: { sheet: 'Hoja1', c1: 0, r1: 0, c2: (m[0]?.length ?? 1) - 1, r2: m.length - 1 },
  };
}

const call = (name: keyof typeof funcs, args: Arg[]): CellValue | CellValue[][] =>
  funcs[name]!(ctx, args);

test('sanidad: los errores son FormulaError, no strings', () => {
  const e = grid();
  expect(isError(cv(e, '=VLOOKUP(9, A1:B4, 2, FALSE)'))).toBe(true);
});
