// Checks de los nombres HEREDADOS de estadística (anteriores a Excel 2010) y de
// la familia moderna CEILING/FLOOR, contra valores conocidos de una hoja real.
//
// El riesgo que cubren no es que la cuenta esté mal —la hacen las funciones
// modernas, ya probadas— sino que el nombre viejo apunte al destino equivocado:
// varias cambiaron de semántica al renombrarse y un sinónimo ingenuo devolvería
// un número plausible y falso. Por eso cada caso fija el valor de Excel, y las
// de cola se comparan además contra su hermana moderna para que no puedan
// coincidir por accidente.
//
// Ejecuta: `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function val(formula: string, cells: Record<string, string> = {}): unknown {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}

function disp(formula: string, cells: Record<string, string> = {}): string {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

const cerca = (f: string, esperado: number, eps = 1e-9) =>
  expect(Math.abs((val(f) as number) - esperado)).toBeLessThan(eps);

test('TDIST es de COLA, no la acumulada de T.DIST', () => {
  // P(T>2) con 10 gl = 0,036694017, comprobado aparte con la beta incompleta
  // regularizada: 0,5·I_{df/(df+t²)}(df/2, 1/2). T.DIST(2;10;TRUE) vale
  // 0,963306 —el complementario—, y confundirlas es el error clásico.
  cerca('=TDIST(2,10,1)', 0.036694017, 1e-8);
  cerca('=TDIST(2,10,2)', 0.073388035, 1e-8);
  expect(val('=TDIST(2,10,1)')).toEqual(val('=T.DIST.RT(2,10)'));
  expect(val('=TDIST(2,10,2)')).toEqual(val('=T.DIST.2T(2,10)'));
  expect(val('=TDIST(2,10,1)')).not.toEqual(val('=T.DIST(2,10,TRUE)'));
  // x negativa y colas fuera de {1,2} son #NUM! en la vieja.
  expect(disp('=TDIST(-1,10,1)')).toBe('#NUM!');
  expect(disp('=TDIST(1,10,3)')).toBe('#NUM!');
});

test('TINV son DOS colas (no T.INV)', () => {
  // Excel: TINV(0,05;10)=2,228139, que es T.INV.2T. T.INV(0,05;10) da -1,812461.
  cerca('=TINV(0.05,10)', 2.22813885, 1e-7);
  expect(val('=TINV(0.05,10)')).toEqual(val('=T.INV.2T(0.05,10)'));
  expect(val('=TINV(0.05,10)')).not.toEqual(val('=T.INV(0.05,10)'));
});

test('FDIST/FINV y CHIDIST/CHIINV son la cola derecha', () => {
  expect(val('=FDIST(1,5,10)')).toEqual(val('=F.DIST.RT(1,5,10)'));
  expect(val('=FINV(0.05,5,10)')).toEqual(val('=F.INV.RT(0.05,5,10)'));
  expect(val('=CHIDIST(3,4)')).toEqual(val('=CHISQ.DIST.RT(3,4)'));
  expect(val('=CHIINV(0.05,4)')).toEqual(val('=CHISQ.INV.RT(0.05,4)'));
  // Y NO la acumulada por la izquierda.
  expect(val('=CHIDIST(3,4)')).not.toEqual(val('=CHISQ.DIST(3,4,TRUE)'));
  cerca('=CHIDIST(3,4)', 0.5578254, 1e-6);
  cerca('=FINV(0.05,5,10)', 3.32583453, 1e-7);
});

test('las que perdieron el argumento «acumulada» lo fijan bien', () => {
  // NORMSDIST y LOGNORMDIST son CDF.
  expect(val('=NORMSDIST(1.96)')).toEqual(val('=NORM.S.DIST(1.96,TRUE)'));
  cerca('=NORMSDIST(1.96)', 0.9750021, 1e-6);
  expect(val('=LOGNORMDIST(4,1,1)')).toEqual(val('=LOGNORM.DIST(4,1,1,TRUE)'));
  // HYPGEOMDIST y NEGBINOMDIST son densidad, no acumulada.
  expect(val('=HYPGEOMDIST(1,4,8,20)')).toEqual(val('=HYPGEOM.DIST(1,4,8,20,FALSE)'));
  expect(val('=HYPGEOMDIST(1,4,8,20)')).not.toEqual(val('=HYPGEOM.DIST(1,4,8,20,TRUE)'));
  expect(val('=NEGBINOMDIST(10,5,0.25)')).toEqual(val('=NEGBINOM.DIST(10,5,0.25,FALSE)'));
  // BETADIST es CDF, y sus límites opcionales se corren tras el «acumulada».
  expect(val('=BETADIST(0.5,2,3)')).toEqual(val('=BETA.DIST(0.5,2,3,TRUE)'));
  expect(val('=BETADIST(5,2,3,0,10)')).toEqual(val('=BETA.DIST(5,2,3,TRUE,0,10)'));
  cerca('=BETADIST(0.5,2,3)', 0.6875, 1e-9);
});

test('las meramente renombradas delegan tal cual', () => {
  expect(val('=NORMDIST(3,2,1,TRUE)')).toEqual(val('=NORM.DIST(3,2,1,TRUE)'));
  expect(val('=NORMINV(0.9,2,1)')).toEqual(val('=NORM.INV(0.9,2,1)'));
  expect(val('=NORMSINV(0.9)')).toEqual(val('=NORM.S.INV(0.9)'));
  expect(val('=EXPONDIST(1,2,TRUE)')).toEqual(val('=EXPON.DIST(1,2,TRUE)'));
  expect(val('=POISSON(2,3,TRUE)')).toEqual(val('=POISSON.DIST(2,3,TRUE)'));
  expect(val('=LOGINV(0.5,1,1)')).toEqual(val('=LOGNORM.INV(0.5,1,1)'));
  expect(val('=GAMMADIST(2,3,1,TRUE)')).toEqual(val('=GAMMA.DIST(2,3,1,TRUE)'));
  expect(val('=GAMMAINV(0.5,3,1)')).toEqual(val('=GAMMA.INV(0.5,3,1)'));
  expect(val('=BETAINV(0.5,2,3)')).toEqual(val('=BETA.INV(0.5,2,3)'));
  expect(val('=CRITBINOM(10,0.5,0.05)')).toEqual(val('=BINOM.INV(10,0.5,0.05)'));
});

test('los contrastes heredados delegan en su versión con punto', () => {
  const m = { A1: '10', A2: '12', A3: '14', B1: '11', B2: '13', B3: '16' };
  expect(val('=TTEST(A1:A3,B1:B3,2,1)', m)).toEqual(val('=T.TEST(A1:A3,B1:B3,2,1)', m));
  expect(val('=FTEST(A1:A3,B1:B3)', m)).toEqual(val('=F.TEST(A1:A3,B1:B3)', m));
  expect(val('=ZTEST(A1:A3,12)', m)).toEqual(val('=Z.TEST(A1:A3,12)', m));
  expect(val('=CHITEST(A1:A3,B1:B3)', m)).toEqual(val('=CHISQ.TEST(A1:A3,B1:B3)', m));
});

test('CEILING/FLOOR modernas: los negativos van respecto a la RECTA', () => {
  // Con positivos coinciden con las clásicas; el negativo es donde se separan.
  expect(val('=CEILING.MATH(6.7)')).toBe(7);
  expect(val('=FLOOR.MATH(6.7)')).toBe(6);
  expect(val('=CEILING.MATH(-4.5,1)')).toBe(-4); // hacia +∞, la clásica daría -5
  expect(val('=FLOOR.MATH(-4.5,1)')).toBe(-5); // hacia -∞
  expect(val('=CEILING.PRECISE(-4.5)')).toBe(-4);
  expect(val('=FLOOR.PRECISE(-4.5)')).toBe(-5);
  expect(val('=ISO.CEILING(-4.5)')).toBe(-4);
  expect(val('=ISO.CEILING(4.42,0.05)')).toBe(4.45);
});

test('CEILING/FLOOR modernas: el «modo» solo afecta a los negativos', () => {
  expect(val('=CEILING.MATH(-4.5,1,1)')).toBe(-5); // aleja del cero
  expect(val('=FLOOR.MATH(-4.5,1,1)')).toBe(-4); // acerca al cero
  // Con positivos el modo es indiferente.
  expect(val('=CEILING.MATH(4.5,1,1)')).toBe(5);
  expect(val('=FLOOR.MATH(4.5,1,1)')).toBe(4);
});

test('CEILING/FLOOR modernas: el signo del múltiplo no da error', () => {
  // La clásica CEILING(-4;-1) exige signos compatibles; estas usan |múltiplo|.
  expect(val('=CEILING.MATH(-4.5,-1)')).toBe(-4);
  expect(val('=FLOOR.MATH(-4.5,-1)')).toBe(-5);
  expect(val('=CEILING.PRECISE(-4.5,-1)')).toBe(-4);
  expect(disp('=CEILING.MATH(4.5,-1)')).not.toBe('#NUM!');
  // Múltiplo 0 → 0, y aquí ni siquiera FLOOR.PRECISE da #DIV/0!.
  expect(val('=CEILING.MATH(4.5,0)')).toBe(0);
  expect(val('=FLOOR.PRECISE(4.5,0)')).toBe(0);
});
