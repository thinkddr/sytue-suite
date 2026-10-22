// Checks de mathtrig: trigonometría, exp/log, combinatoria. Valores conocidos
// de una hoja de cálculo real, más casos de error/dominio. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

/** Evalúa «=…» en A1 y devuelve el valor calculado (RNG inyectable). */
function val(formula: string, random: () => number = () => 0.5): CellValue {
  const e = new Engine({ random });
  e.setCell('Hoja1', 'A1', formula);
  return e.getValue('Hoja1', 'A1');
}

/** Como val() pero exige un número (falla ruidosamente si sale error/texto). */
function num(formula: string, random?: () => number): number {
  const v = val(formula, random);
  if (typeof v !== 'number') throw new Error(`esperaba número, dio ${String(v)}`);
  return v;
}

/** Texto a mostrar: comprueba códigos de error y el formato «General». */
function disp(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  return e.getDisplay('Hoja1', 'A1');
}

test('PI y conversión de ángulos', () => {
  expect(num('=PI()')).toBe(Math.PI);
  expect(disp('=DEGREES(PI())')).toBe('180');
  expect(num('=RADIANS(180)')).toBeCloseTo(Math.PI, 12);
  expect(disp('=PI(1)')).toBe('#VALUE!'); // PI no admite argumentos
});

test('trigonometría directa', () => {
  expect(num('=SIN(0)')).toBe(0);
  expect(num('=COS(0)')).toBe(1);
  expect(num('=TAN(0)')).toBe(0);
  expect(num('=SIN(PI()/6)')).toBeCloseTo(0.5, 12);
  expect(num('=COS(PI())')).toBeCloseTo(-1, 12);
  expect(num('=TAN(PI()/4)')).toBeCloseTo(1, 12);
});

test('trigonometría inversa y su dominio', () => {
  expect(num('=ASIN(1)')).toBeCloseTo(Math.PI / 2, 12);
  expect(num('=ACOS(1)')).toBe(0);
  expect(num('=ATAN(1)')).toBeCloseTo(Math.PI / 4, 12);
  expect(disp('=ASIN(2)')).toBe('#NUM!'); // |x|>1 fuera de dominio
  expect(disp('=ACOS(-2)')).toBe('#NUM!');
});

test('ATAN2 con el orden de Excel (x, y) y el origen', () => {
  expect(num('=ATAN2(1,1)')).toBeCloseTo(Math.PI / 4, 12);
  expect(num('=ATAN2(0,1)')).toBeCloseTo(Math.PI / 2, 12); // eje Y+
  expect(num('=ATAN2(1,0)')).toBe(0); // eje X+
  expect(num('=ATAN2(-1,-1)')).toBeCloseTo((-3 * Math.PI) / 4, 12);
  expect(disp('=ATAN2(0,0)')).toBe('#DIV/0!');
});

test('hiperbólicas y desbordamiento', () => {
  expect(num('=SINH(0)')).toBe(0);
  expect(num('=COSH(0)')).toBe(1);
  expect(num('=TANH(0)')).toBe(0);
  expect(num('=SINH(1)')).toBeCloseTo(1.1752011936438014, 12);
  expect(num('=COSH(1)')).toBeCloseTo(1.5430806348152437, 12);
  expect(num('=TANH(1)')).toBeCloseTo(0.7615941559557649, 12);
  expect(disp('=COSH(1000)')).toBe('#NUM!'); // desborda a infinito
});

test('exponencial y logaritmos', () => {
  expect(num('=EXP(0)')).toBe(1);
  expect(num('=EXP(1)')).toBeCloseTo(Math.E, 12);
  expect(num('=LN(1)')).toBe(0);
  expect(num('=LN(EXP(1))')).toBeCloseTo(1, 12);
  expect(disp('=LOG10(1000)')).toBe('3');
  expect(disp('=LOG(8,2)')).toBe('3');
  expect(disp('=LOG(100)')).toBe('2'); // base por defecto 10
  expect(disp('=EXP(1000)')).toBe('#NUM!');
  expect(disp('=LN(0)')).toBe('#NUM!');
  expect(disp('=LN(-1)')).toBe('#NUM!');
  expect(disp('=LOG(-5)')).toBe('#NUM!');
  expect(disp('=LOG(10,1)')).toBe('#DIV/0!'); // ln(1)=0
});

test('factoriales', () => {
  expect(num('=FACT(0)')).toBe(1);
  expect(num('=FACT(5)')).toBe(120);
  expect(num('=FACT(5.9)')).toBe(120); // trunca a 5
  expect(disp('=FACT(-1)')).toBe('#NUM!');
  expect(num('=FACT(170)')).toBeGreaterThan(0); // último finito
  expect(disp('=FACT(171)')).toBe('#NUM!'); // desborda
  expect(num('=FACTDOUBLE(6)')).toBe(48); // 6·4·2
  expect(num('=FACTDOUBLE(7)')).toBe(105); // 7·5·3·1
  expect(num('=FACTDOUBLE(0)')).toBe(1);
  expect(num('=FACTDOUBLE(-1)')).toBe(1);
  expect(disp('=FACTDOUBLE(-2)')).toBe('#NUM!');
});

test('combinaciones y permutaciones', () => {
  expect(num('=COMBIN(5,2)')).toBe(10);
  expect(num('=COMBIN(10,3)')).toBe(120);
  expect(num('=COMBIN(4,4)')).toBe(1);
  expect(num('=COMBIN(52,5)')).toBe(2598960); // manos de póker
  expect(disp('=COMBIN(3,5)')).toBe('#NUM!'); // k>n
  expect(num('=PERMUT(5,2)')).toBe(20);
  expect(num('=PERMUT(10,3)')).toBe(720);
  expect(num('=PERMUT(4,0)')).toBe(1);
  expect(disp('=PERMUT(3,5)')).toBe('#NUM!');
});

test('RAND y RANDBETWEEN usan el RNG inyectable', () => {
  expect(num('=RAND()')).toBe(0.5);
  expect(num('=RAND()', () => 0)).toBe(0);
  expect(num('=RANDBETWEEN(1,10)')).toBe(6); // 1 + trunc(0.5·10)
  expect(num('=RANDBETWEEN(1,10)', () => 0)).toBe(1);
  expect(num('=RANDBETWEEN(1,10)', () => 0.99)).toBe(10);
  expect(num('=RANDBETWEEN(-3,-1)')).toBe(-2);
  expect(disp('=RANDBETWEEN(5,1)')).toBe('#NUM!'); // bottom > top
});

test('propagación de errores y coacción', () => {
  expect(disp('=SIN(1/0)')).toBe('#DIV/0!'); // error de un subárbol
  expect(disp('=SIN("abc")')).toBe('#VALUE!'); // texto no numérico
  expect(disp('=SIN(1,2)')).toBe('#VALUE!'); // aridad incorrecta

  const e = new Engine();
  e.setCell('Hoja1', 'B2', '=COS(A1)'); // A1 vacía ≈ 0 → COS(0)=1
  expect(e.getValue('Hoja1', 'B2')).toBe(1);

  e.setCell('Hoja1', 'C1', '1');
  e.setCell('Hoja1', 'C2', '2');
  e.setCell('Hoja1', 'C3', '=SIN(C1:C2)'); // rango multicelda escalar → #VALUE!
  expect(e.getDisplay('Hoja1', 'C3')).toBe('#VALUE!');

  e.setCell('Hoja1', 'D1', '=1/0');
  e.setCell('Hoja1', 'D2', '=COMBIN(D1,2)'); // el error llega por referencia
  expect(e.getDisplay('Hoja1', 'D2')).toBe('#DIV/0!');
});

// ---- Endurecimiento: casos límite que faltaban por cubrir ----

test('ATAN2 cubre los cuadrantes Q2/Q4 (signo del orden de Excel)', () => {
  // Q1 y Q3 ya están arriba; aquí los que faltaban, donde el signo delata un
  // intercambio erróneo de argumentos: ATAN2(x;y) = atan2(y, x).
  expect(num('=ATAN2(-1,1)')).toBeCloseTo((3 * Math.PI) / 4, 12); // Q2: x<0, y>0
  expect(num('=ATAN2(1,-1)')).toBeCloseTo(-Math.PI / 4, 12); // Q4: x>0, y<0
  expect(num('=ATAN2(-1,0)')).toBeCloseTo(Math.PI, 12); // eje X−
  expect(num('=ATAN2(0,-1)')).toBeCloseTo(-Math.PI / 2, 12); // eje Y−
  expect(disp('=ATAN2(1)')).toBe('#VALUE!'); // aridad: falta un argumento
});

test('el guardián de #NUM! no marca los resultados finitos grandes', () => {
  // map1 mapea a #NUM! solo lo NO finito; un resultado grande pero finito pasa
  // (protege contra "acotar el dominio" y romper la saturación/subdesbordamiento).
  expect(num('=TANH(1000)')).toBe(1); // satura a 1, no es #NUM!
  expect(num('=EXP(-1000)')).toBe(0); // subdesborda a 0 (finito), no #NUM!
  expect(num('=ATAN(1E300)')).toBeCloseTo(Math.PI / 2, 12);
});

test('COMBIN/PERMUT: simetría, extremos, desbordamiento y aridad', () => {
  expect(num('=COMBIN(0,0)')).toBe(1);
  expect(num('=COMBIN(8,0)')).toBe(1);
  expect(num('=COMBIN(8,8)')).toBe(1);
  expect(num('=COMBIN(8,3)')).toBe(num('=COMBIN(8,5)')); // C(n,k)=C(n,n−k)
  expect(num('=COMBIN(30,15)')).toBe(155117520); // exacto, sin ruido de f64
  expect(disp('=COMBIN(1030,515)')).toBe('#NUM!'); // desborda a ∞
  expect(disp('=COMBIN(5)')).toBe('#VALUE!'); // aridad
  expect(num('=PERMUT(0,0)')).toBe(1);
  expect(num('=PERMUT(5,5)')).toBe(120); // = 5!
  expect(disp('=PERMUT(5,1,1)')).toBe('#VALUE!'); // aridad
});

test('LOG con base fraccionaria, cero, negativa y por defecto', () => {
  expect(num('=LOG(0.5,0.5)')).toBe(1); // log_0.5(0.5) = 1
  expect(num('=LOG(1)')).toBe(0); // base 10 por defecto
  expect(disp('=LOG(8,0)')).toBe('#NUM!'); // base 0 fuera de dominio
  expect(disp('=LOG(8,-2)')).toBe('#NUM!'); // base negativa
  expect(disp('=LOG()')).toBe('#VALUE!'); // aridad
});

test('RANDBETWEEN: punto único y límites fraccionarios (ceil abajo, floor arriba)', () => {
  expect(num('=RANDBETWEEN(7,7)')).toBe(7); // lo == hi
  expect(num('=RANDBETWEEN(1.5,5.5)')).toBe(4); // ceil 2 .. floor 5, 2 + trunc(0.5·4)
  expect(num('=RANDBETWEEN(1.5,5.5)', () => 0)).toBe(2); // límite inferior = ceil(1.5)
});
