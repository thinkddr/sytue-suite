// Checks de la familia «math» (SUM, ROUND, GCD, …) contra valores conocidos de
// una hoja de cálculo real. Extremo a extremo por el motor. Ejecuta: `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

/** Evalúa `formula` en Z1 con las celdas `cells` fijadas antes. Valor calculado. */
function val(formula: string, cells: Record<string, string> = {}): unknown {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}

/** Igual que `val` pero devuelve el texto a mostrar (errores y decimales limpios). */
function disp(formula: string, cells: Record<string, string> = {}): string {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('SUM: rangos, escalares, booleano directo y propagación', () => {
  expect(val('=SUM(1,2,3)')).toBe(6);
  expect(val('=SUM(A1:A3)', { A1: '1', A2: '2', A3: '3' })).toBe(6);
  // Booleano cuenta como escalar directo (1), no dentro de un rango.
  expect(val('=SUM(A1:A2,TRUE)', { A1: '5', A2: 'texto' })).toBe(6);
  expect(val('=SUM("3",2)')).toBe(5); // el texto numérico escrito como argumento se convierte
  expect(disp('=SUM(A1:A2)', { A1: '1', A2: '=1/0' })).toBe('#DIV/0!');
});

test('PRODUCT: producto; sin números → 0', () => {
  expect(val('=PRODUCT(2,3,4)')).toBe(24);
  expect(val('=PRODUCT(A1:A3)', { A1: '2', A2: '3', A3: '4' })).toBe(24);
  expect(val('=PRODUCT(A1:A3,5)', { A1: '2' })).toBe(10); // blancos ignorados
  expect(val('=PRODUCT(A1:A3)')).toBe(0); // rango vacío → 0
  expect(val('=PRODUCT(5,TRUE)')).toBe(5);
});

test('SUMSQ y SUMPRODUCT', () => {
  expect(val('=SUMSQ(3,4)')).toBe(25);
  expect(val('=SUMSQ(A1:A2)', { A1: '3', A2: '4' })).toBe(25);
  const cells = { A1: '1', A2: '2', A3: '3', B1: '4', B2: '5', B3: '6' };
  expect(val('=SUMPRODUCT(A1:A3,B1:B3)', cells)).toBe(32);
  expect(val('=SUMPRODUCT(A1:A3)', { A1: '1', A2: '2', A3: '3' })).toBe(6);
  // Tamaños distintos → #VALUE!.
  expect(disp('=SUMPRODUCT(A1:A3,B1:B2)', cells)).toBe('#VALUE!');
});

test('ABS / SIGN', () => {
  expect(val('=ABS(-7)')).toBe(7);
  expect(val('=ABS(7)')).toBe(7);
  expect(disp('=ABS(1/0)')).toBe('#DIV/0!'); // el #error se propaga
  expect(disp('=ABS(A1:A2)', { A1: '1', A2: '2' })).toBe('#VALUE!'); // rango en contexto escalar
  expect(val('=SIGN(-5)')).toBe(-1);
  expect(val('=SIGN(0)')).toBe(0);
  expect(val('=SIGN(10)')).toBe(1);
});

test('MOD: signo del divisor; QUOTIENT: trunca', () => {
  expect(val('=MOD(10,3)')).toBe(1);
  expect(val('=MOD(-3,2)')).toBe(1);
  expect(val('=MOD(3,-2)')).toBe(-1);
  expect(disp('=MOD(5,0)')).toBe('#DIV/0!');
  expect(val('=QUOTIENT(7,2)')).toBe(3);
  expect(val('=QUOTIENT(-7,2)')).toBe(-3);
  expect(disp('=QUOTIENT(5,0)')).toBe('#DIV/0!');
});

test('POWER / SQRT: dominio → #NUM!', () => {
  expect(val('=POWER(2,10)')).toBe(1024);
  expect(val('=POWER(-2,2)')).toBe(4);
  expect(val('=POWER(2,0.5)')).toBeCloseTo(Math.SQRT2, 12);
  expect(disp('=POWER(-8,1/3)')).toBe('#NUM!'); // base negativa, exponente fraccionario
  expect(val('=SQRT(16)')).toBe(4);
  expect(disp('=SQRT(-1)')).toBe('#NUM!');
});

test('ROUND / ROUNDUP / ROUNDDOWN: mitad hacia afuera y ruido f64', () => {
  expect(val('=ROUND(2.5,0)')).toBe(3);
  expect(val('=ROUND(-2.5,0)')).toBe(-3);
  expect(disp('=ROUND(2.15,1)')).toBe('2.2'); // ruido binario corregido
  expect(disp('=ROUND(1.005,2)')).toBe('1.01'); // caso clásico
  expect(val('=ROUND(1234.5678,-2)')).toBe(1200);
  expect(val('=ROUNDUP(3.2,0)')).toBe(4);
  expect(val('=ROUNDUP(-3.2,0)')).toBe(-4);
  expect(disp('=ROUNDUP(3.14159,2)')).toBe('3.15');
  expect(val('=ROUNDDOWN(3.7,0)')).toBe(3);
  expect(val('=ROUNDDOWN(-3.7,0)')).toBe(-3);
  expect(disp('=ROUNDDOWN(3.14159,2)')).toBe('3.14');
});

test('INT (hacia −∞) vs TRUNC (hacia cero)', () => {
  expect(val('=INT(8.9)')).toBe(8);
  expect(val('=INT(-8.9)')).toBe(-9);
  expect(val('=TRUNC(8.9)')).toBe(8);
  expect(val('=TRUNC(-8.9)')).toBe(-8);
  expect(disp('=TRUNC(8.912,2)')).toBe('8.91');
  expect(val('=TRUNC(31.5,-1)')).toBe(30);
});

test('MROUND: múltiplo más cercano; signos opuestos → #NUM!', () => {
  expect(val('=MROUND(10,3)')).toBe(9);
  expect(val('=MROUND(11,3)')).toBe(12);
  expect(disp('=MROUND(1.3,0.2)')).toBe('1.4');
  expect(val('=MROUND(-10,-3)')).toBe(-9);
  expect(val('=MROUND(5,0)')).toBe(0);
  expect(disp('=MROUND(5,-2)')).toBe('#NUM!');
});

test('CEILING / FLOOR: múltiplo, signos y significance por defecto', () => {
  expect(val('=CEILING(2.5,1)')).toBe(3);
  expect(val('=CEILING(-2.5,-2)')).toBe(-4);
  expect(val('=CEILING(-2.5,2)')).toBe(-2);
  expect(disp('=CEILING(0.234,0.01)')).toBe('0.24');
  expect(val('=CEILING(2.5)')).toBe(3); // significance por defecto = 1
  expect(disp('=CEILING(2.5,-2)')).toBe('#NUM!');
  expect(val('=FLOOR(2.5,1)')).toBe(2);
  expect(val('=FLOOR(-2.5,-2)')).toBe(-2);
  expect(disp('=FLOOR(0.234,0.01)')).toBe('0.23');
  expect(val('=FLOOR(2.5)')).toBe(2);
  expect(disp('=FLOOR(2.5,-2)')).toBe('#NUM!');
  expect(disp('=FLOOR(5,0)')).toBe('#DIV/0!');
});

test('GCD / LCM: enteros; 0 y negativos', () => {
  expect(val('=GCD(24,36)')).toBe(12);
  expect(val('=GCD(5,0)')).toBe(5);
  expect(val('=GCD(7.5,5)')).toBe(1); // trunca 7,5 → 7
  expect(val('=GCD(A1:A3)', { A1: '12', A2: '18', A3: '24' })).toBe(6);
  expect(disp('=GCD(-4,8)')).toBe('#NUM!');
  expect(val('=LCM(4,6)')).toBe(12);
  expect(val('=LCM(24,36)')).toBe(72);
  expect(val('=LCM(5,0)')).toBe(0);
  expect(disp('=LCM(-4,6)')).toBe('#NUM!');
});

/* ---------- Endurecimiento (verificador): casos límite y propagación ---------- */

test('propagación del #error en los agregados, no solo en SUM', () => {
  // Solo SUM tenía cobertura de propagación; SUMSQ/PRODUCT/SUMPRODUCT también.
  expect(disp('=SUMSQ(A1:A2)', { A1: '3', A2: '=1/0' })).toBe('#DIV/0!');
  expect(disp('=PRODUCT(A1:A2)', { A1: '3', A2: '=1/0' })).toBe('#DIV/0!');
  expect(disp('=SUMPRODUCT(A1:A2,B1:B2)', { A1: '1', A2: '2', B1: '=SQRT(-1)', B2: '4' })).toBe(
    '#NUM!',
  );
  // Producto escalar correcto con matrices 1×2 (fila): 1·3 + 2·4 = 11.
  expect(val('=SUMPRODUCT(A1:B1,A2:B2)', { A1: '1', B1: '2', A2: '3', B2: '4' })).toBe(11);
});

test('ROUND: mitad hacia afuera también con negativos y dígitos negativos', () => {
  expect(val('=ROUND(-0.5,0)')).toBe(-1); // media negativa → aleja de cero
  expect(val('=ROUND(250,-2)')).toBe(300); // 2,5 centenas → 3 (media hacia afuera)
  expect(val('=ROUND(150,-2)')).toBe(200);
  expect(val('=TRUNC(-31.5,-1)')).toBe(-30); // trunca hacia cero con dígitos negativos
  expect(val('=QUOTIENT(10,-3)')).toBe(-3); // trunca hacia cero (divisor negativo)
});

test('MROUND: la limpieza de ruido f64 sobrevive a los múltiplos fraccionarios', () => {
  // 6,05/0,1 = 60,4999…; sin la limpieza a 15 cifras redondearía a 60 → 6,0 (mal).
  expect(disp('=MROUND(6.05,0.1)')).toBe('6.1');
  expect(disp('=MROUND(7.05,0.1)')).toBe('7.1');
  expect(disp('=MROUND(1.25,0.5)')).toBe('1.5'); // 2,5 → 3 (media hacia afuera) → 1,5
});

test('CEILING / FLOOR: matriz de signos completa y múltiplo exacto', () => {
  // Ambos negativos → se aleja de cero (hacia −∞).
  expect(val('=CEILING(-0.5,-2)')).toBe(-2);
  expect(val('=FLOOR(-0.5,-2)')).toBe(0); // hacia cero
  expect(val('=CEILING(-2.7,-0.5)')).toBe(-3);
  expect(disp('=FLOOR(-2.7,-0.5)')).toBe('-2.5');
  // Número negativo con significancia positiva: hacia ±∞ (convención FLOOR.MATH/OpenFormula).
  expect(val('=FLOOR(-2.5,2)')).toBe(-4);
  // Múltiplo exacto: no debe «pasarse» por ruido binario (6/3 = 2 justo).
  expect(val('=CEILING(6,3)')).toBe(6);
  expect(val('=FLOOR(6,3)')).toBe(6);
});

test('POWER / GCD / LCM: bordes de dominio y multiargumento', () => {
  expect(val('=POWER(0,0)')).toBe(1); // 0⁰ = 1 (como el operador «^»)
  expect(disp('=POWER(0,-1)')).toBe('#NUM!'); // no finito → #NUM!
  expect(val('=GCD(12,8,20)')).toBe(4); // más de dos argumentos
  expect(val('=LCM(2,3,4)')).toBe(12);
  expect(val('=GCD(0,0)')).toBe(0); // por definición, MCD(0,0)=0
});
