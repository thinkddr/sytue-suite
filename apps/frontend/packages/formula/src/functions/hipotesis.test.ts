/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pruebas de hipótesis (T.TEST, F.TEST, CHISQ.TEST), dispersión sobre base de datos
 * (DSTDEV/DSTDEVP/DVAR/DVARP) y bases numéricas (BASE, DECIMAL, ROMAN, ARABIC).
 *
 * El hilo común de estos casos: todos son funciones que ante un uso incorrecto pueden
 * devolver un número perfectamente creíble. Un T.TEST con el tipo equivocado, una
 * varianza poblacional donde tocaba la muestral o un DECIMAL que se traga los dígitos
 * que no entiende no fallan — dan otra cosa, y nadie lo nota.
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function ev(formula: string, celdas: Record<string, string | number> = {}): CellValue | string {
  const e = new Engine();
  for (const [ref, v] of Object.entries(celdas)) e.setCell('Hoja1', ref, String(v));
  e.setCell('Hoja1', 'Z1', formula);
  const v = e.getValue('Hoja1', 'Z1');
  return typeof v === 'object' && v !== null && 'code' in v ? (v as { code: string }).code : v;
}

/* ---------- Dispersión sobre registros filtrados ---------- */

/** Tabla mínima: una columna «v» con criterio que deja pasar los cuatro registros. */
const TABLA = {
  A1: 'v',
  A2: 2,
  A3: 4,
  A4: 4,
  A5: 6,
  C1: 'v',
  C2: '>0',
};

test('la variante P es POBLACIONAL y la otra MUESTRAL: no son la misma cuenta', () => {
  // {2,4,4,6}: media 4, Σ(x−x̄)² = 8.
  // Poblacional: 8/4 = 2 → desviación √2. Muestral: 8/3 → desviación √(8/3).
  // Confundirlas da un resultado creíble y sesgado siempre en el mismo sentido.
  expect(ev('=DVARP(A1:A5,"v",C1:C2)', TABLA) as number).toBeCloseTo(2, 10);
  expect(ev('=DVAR(A1:A5,"v",C1:C2)', TABLA) as number).toBeCloseTo(8 / 3, 10);
  expect(ev('=DSTDEVP(A1:A5,"v",C1:C2)', TABLA) as number).toBeCloseTo(Math.sqrt(2), 10);
  expect(ev('=DSTDEV(A1:A5,"v",C1:C2)', TABLA) as number).toBeCloseTo(Math.sqrt(8 / 3), 10);
});

test('con un solo registro la muestral no existe, pero la poblacional vale 0', () => {
  // n−1 = 0: no hay varianza muestral que estimar. La poblacional, en cambio, está
  // perfectamente definida. Esa asimetría distingue las dos familias.
  const una = { A1: 'v', A2: 5, C1: 'v', C2: '>0' };
  expect(ev('=DVAR(A1:A2,"v",C1:C2)', una)).toBe('#DIV/0!');
  expect(ev('=DVARP(A1:A2,"v",C1:C2)', una)).toBe(0);
});

/* ---------- Bases numéricas ---------- */

test('BASE y DECIMAL son inversas, con relleno opcional', () => {
  expect(ev('=BASE(255,16)')).toBe('FF');
  expect(ev('=BASE(255,2)')).toBe('11111111');
  expect(ev('=BASE(7,2,8)')).toBe('00000111'); // rellenado a 8
  expect(ev('=DECIMAL("FF",16)')).toBe(255);
  expect(ev('=DECIMAL("11111111",2)')).toBe(255);
  expect(ev('=DECIMAL("zz",36)')).toBe(1295); // minúsculas admitidas
});

test('DECIMAL rechaza los dígitos ajenos a la base en vez de tragárselos', () => {
  // `parseInt("1Z", 2)` devuelve 1: corta en el primer carácter inválido y no avisa.
  // Aquí eso sería un número plausible salido de un dato corrupto.
  expect(ev('=DECIMAL("1Z",2)')).toBe('#NUM!');
  expect(ev('=DECIMAL("129",8)')).toBe('#NUM!'); // el 9 no existe en octal
  expect(ev('=BASE(-1,2)')).toBe('#NUM!'); // sin convenio de signo en base arbitraria
  expect(ev('=BASE(10,1)')).toBe('#NUM!'); // base fuera de 2..36
  expect(ev('=BASE(10,37)')).toBe('#NUM!');
});

test('ROMAN usa las formas sustractivas, no cuatro palotes', () => {
  // Si las formas IV/IX/XL van en la tabla, la conversión es un recorrido codicioso.
  // Parchearlas después con reemplazos de texto es lo que produce «IIII».
  expect(ev('=ROMAN(4)')).toBe('IV');
  expect(ev('=ROMAN(9)')).toBe('IX');
  expect(ev('=ROMAN(40)')).toBe('XL');
  expect(ev('=ROMAN(1994)')).toBe('MCMXCIV');
  expect(ev('=ROMAN(3999)')).toBe('MMMCMXCIX');
  expect(ev('=ROMAN(4000)')).toBe('#NUM!'); // fuera del sistema clásico
});

test('ARABIC deshace ROMAN, incluida la resta', () => {
  expect(ev('=ARABIC("IV")')).toBe(4);
  expect(ev('=ARABIC("MCMXCIV")')).toBe(1994);
  expect(ev('=ARABIC("XLII")')).toBe(42);
  // Ida y vuelta sobre un valor con varias restas encadenadas.
  expect(ev('=ARABIC(ROMAN(1949))')).toBe(1949);
  expect(ev('=ARABIC("XYZ")')).toBe('#VALUE!');
});

/* ---------- Pruebas de hipótesis ---------- */

const MUESTRAS = {
  A1: 3,
  A2: 4,
  A3: 5,
  A4: 8,
  A5: 9,
  B1: 1,
  B2: 2,
  B3: 4,
  B4: 5,
  B5: 8,
};

test('los tres tipos de T.TEST dan resultados DISTINTOS: no son intercambiables', () => {
  // Aplicar el pareado a muestras independientes no falla, da otro valor P. Por eso el
  // tipo es obligatorio y no tiene defecto: adivinar la intención aquí sale caro.
  const pareada = ev('=T.TEST(A1:A5,B1:B5,2,1)', MUESTRAS) as number;
  const igualVar = ev('=T.TEST(A1:A5,B1:B5,2,2)', MUESTRAS) as number;
  const welch = ev('=T.TEST(A1:A5,B1:B5,2,3)', MUESTRAS) as number;
  for (const p of [pareada, igualVar, welch]) {
    expect(p).toBeGreaterThan(0);
    expect(p).toBeLessThan(1);
  }
  expect(pareada).not.toBeCloseTo(igualVar, 6);
});

test('dos colas es exactamente el doble que una', () => {
  const una = ev('=T.TEST(A1:A5,B1:B5,1,2)', MUESTRAS) as number;
  const dos = ev('=T.TEST(A1:A5,B1:B5,2,2)', MUESTRAS) as number;
  expect(dos).toBeCloseTo(2 * una, 10);
});

test('T.TEST valida el tipo y la longitud en la prueba pareada', () => {
  expect(ev('=T.TEST(A1:A5,B1:B5,3,1)', MUESTRAS)).toBe('#NUM!'); // colas ∉ {1,2}
  expect(ev('=T.TEST(A1:A5,B1:B5,2,4)', MUESTRAS)).toBe('#NUM!'); // tipo ∉ {1,2,3}
  // La pareada trabaja sobre diferencias: sin pares no hay prueba.
  const desigual = { ...MUESTRAS, B5: '' };
  expect(ev('=T.TEST(A1:A5,B1:B4,2,1)', desigual)).toBe('#N/A');
});

test('F.TEST no depende del orden de las muestras', () => {
  const ab = ev('=F.TEST(A1:A5,B1:B5)', MUESTRAS) as number;
  const ba = ev('=F.TEST(B1:B5,A1:A5)', MUESTRAS) as number;
  // Poner la varianza mayor siempre en el numerador es lo que da esta simetría; sin
  // eso, intercambiar los argumentos devuelve la cola contraria.
  expect(ab).toBeCloseTo(ba, 10);
  expect(ab).toBeGreaterThan(0);
  expect(ab).toBeLessThanOrEqual(2);
});

test('CHISQ.TEST saca los grados de libertad de la FORMA de la tabla', () => {
  // Vector 1×4: df = 3, no 4. Observadas iguales a esperadas → χ² = 0 → P = 1.
  const iguales = { A1: 10, B1: 10, C1: 10, D1: 10, A3: 10, B3: 10, C3: 10, D3: 10 };
  expect(ev('=CHISQ.TEST(A1:D1,A3:D3)', iguales) as number).toBeCloseTo(1, 10);

  // Con desviación real hay que fijar el VALOR, no solo que sea pequeño: observadas
  // {20,10,5,5} contra esperadas {10,10,10,10} dan χ² = (100+0+25+25)/10 = 15, y con
  // df = 3 el valor P es ≈ 0.00182. Usando df = 4 —el número de celdas, que es el error
  // habitual— saldría ≈ 0.00473: también «pequeño», así que un test que solo pida
  // «menor que 0.05» no distingue una implementación de la otra.
  const distintas = { A1: 20, B1: 10, C1: 5, D1: 5, A3: 10, B3: 10, C3: 10, D3: 10 };
  expect(ev('=CHISQ.TEST(A1:D1,A3:D3)', distintas) as number).toBeCloseTo(0.001817, 5);
});

test('CHISQ.TEST no divide por una frecuencia esperada nula', () => {
  // Una esperada de 0 hace infinito el término; es un error de datos, no un P bajísimo.
  const conCero = { A1: 5, B1: 5, A3: 0, B3: 10 };
  expect(ev('=CHISQ.TEST(A1:B1,A3:B3)', conCero)).toBe('#DIV/0!');
  // Y las tablas de formas distintas no se comparan.
  const desigual = { A1: 5, B1: 5, A3: 5 };
  expect(ev('=CHISQ.TEST(A1:B1,A3:A3)', desigual)).toBe('#N/A');
});
