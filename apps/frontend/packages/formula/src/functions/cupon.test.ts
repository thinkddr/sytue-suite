/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Calendario de cupones (COUP*), intereses devengados (ACCRINT/ACCRINTM),
 * valores con descuento (PRICEDISC/YIELDDISC) e ISPMT.
 *
 * Los valores de referencia son los EJEMPLOS DOCUMENTADOS de la hoja de cálculo
 * (liquidación 25-ene-2011 / vencimiento 15-nov-2011 para la familia COUP*, y
 * los casos de 2008 para el resto), no lo que devuelve esta implementación. Los
 * que no vienen de un ejemplo publicado se comprueban a mano en el comentario.
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function num(formula: string): number {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  return e.getValue('Hoja1', 'A1') as number;
}
function show(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  return e.getDisplay('Hoja1', 'A1');
}

// Liquidación 25-ene-2011, vencimiento 15-nov-2011, semestral. Los cupones son
// el 15 de mayo y el 15 de noviembre, así que el periodo en curso va del
// 15-nov-2010 al 15-may-2011.
const BONO = 'DATE(2011,1,25),DATE(2011,11,15),2';

test('familia COUP* con base 1 (real/real): ejemplo documentado', () => {
  expect(num(`=COUPDAYBS(${BONO},1)`)).toBe(71); // 15-nov-2010 → 25-ene-2011
  expect(num(`=COUPDAYS(${BONO},1)`)).toBe(181); // 15-nov-2010 → 15-may-2011
  expect(num(`=COUPDAYSNC(${BONO},1)`)).toBe(110); // 25-ene-2011 → 15-may-2011
  expect(num(`=COUPNUM(${BONO},1)`)).toBe(2); // quedan el de mayo y el de noviembre
  expect(num(`=COUPPCD(${BONO},1)`)).toBe(num('=DATE(2010,11,15)'));
  expect(num(`=COUPNCD(${BONO},1)`)).toBe(num('=DATE(2011,5,15)'));
  // Con base real/real las tres cuentas cuadran; es la única base en que lo hacen.
  expect(num(`=COUPDAYBS(${BONO},1)`) + num(`=COUPDAYSNC(${BONO},1)`)).toBe(
    num(`=COUPDAYS(${BONO},1)`),
  );
});

test('la base cambia el recuento de días, no las fechas de cupón', () => {
  // Base 0 (30/360 US): del 15-nov al 25-ene son 2 meses de 30 días + 10 = 70,
  // y el periodo completo son 360/2 = 180 por definición, no 181.
  expect(num(`=COUPDAYBS(${BONO},0)`)).toBe(70);
  expect(num(`=COUPDAYS(${BONO},0)`)).toBe(180);
  expect(num(`=COUPDAYS(${BONO})`)).toBe(180); // la base 0 es la de por defecto
  // Base 2 (real/360): los días transcurridos son REALES (71) pero el periodo
  // sigue siendo nominal (180). Que 71 + 110 ≠ 180 no es un fallo: es la
  // convención, y por eso COUPDAYSNC no se calcula como COUPDAYS − COUPDAYBS.
  expect(num(`=COUPDAYBS(${BONO},2)`)).toBe(71);
  expect(num(`=COUPDAYS(${BONO},2)`)).toBe(180);
  // Base 3 (real/365): el periodo nominal es 365/2 y sí sale fraccionario.
  expect(num(`=COUPDAYS(${BONO},3)`)).toBe(182.5);
  // Las fechas no dependen de la base.
  for (const base of [0, 1, 2, 3, 4]) {
    expect(num(`=COUPPCD(${BONO},${base})`)).toBe(num('=DATE(2010,11,15)'));
    expect(num(`=COUPNCD(${BONO},${base})`)).toBe(num('=DATE(2011,5,15)'));
  }
});

test('el calendario se ancla en el vencimiento y respeta el fin de mes', () => {
  // Vencimiento el 30-jun (último día de junio): los cupones caen en el ÚLTIMO
  // día de cada mes, así que el de diciembre es el 31 y no el 30. Encadenando
  // «mismo día del mes» saldría 30-dic y el devengado tendría un día de menos
  // todos los años.
  expect(num('=COUPNCD(DATE(2014,10,1),DATE(2015,6,30),2)')).toBe(num('=DATE(2014,12,31)'));
  expect(num('=COUPPCD(DATE(2014,10,1),DATE(2015,6,30),2)')).toBe(num('=DATE(2014,6,30)'));
  // Vencimiento el 31-ago con paso semestral: febrero se recorta al último día
  // que existe, 29 en 2012 (bisiesto) y 28 en 2013.
  expect(num('=COUPNCD(DATE(2012,1,15),DATE(2015,8,31),2)')).toBe(num('=DATE(2012,2,29)'));
  expect(num('=COUPNCD(DATE(2013,1,15),DATE(2015,8,31),2)')).toBe(num('=DATE(2013,2,28)'));
  // Vencimiento el 30 (que NO es fin de mes en agosto): el recorte de febrero no
  // se propaga porque cada fecha se genera desde el vencimiento, no de la anterior.
  expect(num('=COUPNCD(DATE(2012,1,15),DATE(2015,8,30),2)')).toBe(num('=DATE(2012,2,29)'));
  expect(num('=COUPNCD(DATE(2012,3,15),DATE(2015,8,30),2)')).toBe(num('=DATE(2012,8,30)'));
});

test('liquidación EXACTAMENTE en una fecha de cupón: el cupón del día ya es el anterior', () => {
  // Quien compra el día del cupón no cobra ese cupón: no hay devengado (0 días)
  // y el periodo en curso es el que empieza ese mismo día.
  expect(num('=COUPPCD(DATE(2011,5,15),DATE(2011,11,15),2)')).toBe(num('=DATE(2011,5,15)'));
  expect(num('=COUPNCD(DATE(2011,5,15),DATE(2011,11,15),2)')).toBe(num('=DATE(2011,11,15)'));
  expect(num('=COUPDAYBS(DATE(2011,5,15),DATE(2011,11,15),2,1)')).toBe(0);
  expect(num('=COUPNUM(DATE(2011,5,15),DATE(2011,11,15),2)')).toBe(1);
});

test('COUP*: frecuencia, orden de fechas y base fuera de rango', () => {
  expect(show(`=COUPNUM(DATE(2011,1,25),DATE(2011,11,15),3)`)).toBe('#NUM!'); // frecuencia 3
  expect(show(`=COUPNUM(DATE(2011,1,25),DATE(2011,11,15),0)`)).toBe('#NUM!');
  expect(show(`=COUPNUM(DATE(2011,11,15),DATE(2011,1,25),2)`)).toBe('#NUM!'); // vence antes
  expect(show(`=COUPDAYS(${BONO},5)`)).toBe('#NUM!'); // base inválida
});

test('ACCRINT: devengado de un valor con cupones periódicos', () => {
  // Ejemplo documentado: emisión 1-mar-2008, primer interés 31-ago-2008,
  // liquidación 1-may-2008, 10 %, nominal 1000, semestral, base 30/360 US.
  // 60 días de 360 sobre 1000 al 10 % = 16,666…
  expect(num('=ACCRINT(DATE(2008,3,1),DATE(2008,8,31),DATE(2008,5,1),0.1,1000,2,0)')).toBeCloseTo(
    16.666667,
    5,
  );
  // Ejemplo documentado con método de cálculo FALSO. Como la liquidación es
  // ANTERIOR al primer interés, el método no se aplica y se devenga igualmente
  // desde la emisión: 26 días de 360 → 7,2222. Aplicarlo daría 0.
  expect(
    num('=ACCRINT(DATE(2008,4,5),DATE(2008,6,30),DATE(2008,5,1),0.1,1000,2,0,FALSE)'),
  ).toBeCloseTo(7.222222, 5);
  expect(
    num('=ACCRINT(DATE(2008,4,5),DATE(2008,6,30),DATE(2008,5,1),0.1,1000,2,0,TRUE)'),
  ).toBeCloseTo(7.222222, 5);
  // Con método FALSO y liquidación POSTERIOR al primer interés sí cuenta: se
  // devenga desde el 30-jun y no desde el 5-abr. Del 30-jun al 31-jul hay 31
  // días reales pero 30 en la base 30/360 US (el día 31 de destino se recorta a
  // 30 cuando el de origen ya es 30), que es la base pedida aquí.
  expect(
    num('=ACCRINT(DATE(2008,4,5),DATE(2008,6,30),DATE(2008,7,31),0.1,1000,2,0,FALSE)'),
  ).toBeCloseTo((1000 * 0.1 * 30) / 360, 9);
  // En base 2 (real/360) sí son los 31 días del calendario.
  expect(
    num('=ACCRINT(DATE(2008,4,5),DATE(2008,6,30),DATE(2008,7,31),0.1,1000,2,2,FALSE)'),
  ).toBeCloseTo((1000 * 0.1 * 31) / 360, 9);
});

test('ACCRINT base 1: reparte por CUASI-CUPONES, no por un periodo medio', () => {
  // Mismo caso que el ejemplo documentado pero en real/real. Los cuasi-cupones
  // se anclan en el primer interés (31-ago, fin de mes), así que el periodo que
  // contiene el devengo va del 29-feb-2008 al 31-ago-2008: son 184 días. Del
  // 1-mar al 1-may hay 61. Devengado = 1000·(0,1/2)·(61/184) = 16,5760869…
  //
  // Repartir sobre un periodo «medio» de 182,5 días daría 16,71: plausible,
  // distinto y falso. Ese es justo el error que este caso caza.
  expect(num('=ACCRINT(DATE(2008,3,1),DATE(2008,8,31),DATE(2008,5,1),0.1,1000,2,1)')).toBeCloseTo(
    (1000 * (0.1 / 2) * 61) / 184,
    9,
  );
  expect(num('=ACCRINT(DATE(2008,3,1),DATE(2008,8,31),DATE(2008,5,1),0.1,1000,2,1)')).toBeCloseTo(
    16.576087,
    5,
  );
});

test('ACCRINTM: sin cupones no hay calendario, solo fracción de año', () => {
  // Ejemplo documentado: emisión 1-abr-2008, liquidación 15-jun-2008, 10 %,
  // nominal 1000, base 3 (real/365). Son 75 días: 1000·0,1·75/365 = 20,54794…
  expect(num('=ACCRINTM(DATE(2008,4,1),DATE(2008,6,15),0.1,1000,3)')).toBeCloseTo(20.547945, 5);
  expect(num('=ACCRINTM(DATE(2008,4,1),DATE(2008,6,15),0.1,1000,3)')).toBeCloseTo(
    (1000 * 0.1 * 75) / 365,
    9,
  );
});

test('ACCRINT / ACCRINTM: dominios', () => {
  const base = 'DATE(2008,3,1),DATE(2008,8,31),DATE(2008,5,1)';
  expect(show(`=ACCRINT(${base},0,1000,2)`)).toBe('#NUM!'); // tasa 0
  expect(show(`=ACCRINT(${base},0.1,0,2)`)).toBe('#NUM!'); // nominal 0
  expect(show(`=ACCRINT(${base},0.1,1000,3)`)).toBe('#NUM!'); // frecuencia 3
  expect(show('=ACCRINT(DATE(2008,5,1),DATE(2008,8,31),DATE(2008,3,1),0.1,1000,2)')).toBe('#NUM!'); // liquida antes de emitir
  expect(show('=ACCRINTM(DATE(2008,6,15),DATE(2008,4,1),0.1,1000)')).toBe('#NUM!');
  expect(show('=ACCRINTM(DATE(2008,4,1),DATE(2008,6,15),0.1,1000,7)')).toBe('#NUM!'); // base inválida
});

test('PRICEDISC / YIELDDISC: valores emitidos al descuento', () => {
  // Ejemplo documentado: liquidación 16-feb-2008, vencimiento 1-mar-2008,
  // descuento 5,25 %, rescate 100, base 2 (real/360). Son 14 días reales.
  expect(num('=PRICEDISC(DATE(2008,2,16),DATE(2008,3,1),0.0525,100,2)')).toBeCloseTo(99.7958333, 6);
  expect(num('=PRICEDISC(DATE(2008,2,16),DATE(2008,3,1),0.0525,100,2)')).toBeCloseTo(
    100 - (0.0525 * 100 * 14) / 360,
    9,
  );
  // YIELDDISC es su inversa: parte del precio y devuelve el rendimiento anual.
  // (0,205/99,795) anualizado por 360/14 = 0,05282257…; el ejemplo publicado lo
  // muestra redondeado a 0,052822.
  expect(num('=YIELDDISC(DATE(2008,2,16),DATE(2008,3,1),99.795,100,2)')).toBeCloseTo(
    (0.205 / 99.795) * (360 / 14),
    9,
  );
  const pr = num('=PRICEDISC(DATE(2008,2,16),DATE(2008,3,1),0.0525,100,2)');
  expect(num(`=YIELDDISC(DATE(2008,2,16),DATE(2008,3,1),${pr},100,2)`)).toBeCloseTo(
    (100 / pr - 1) * (360 / 14),
    9,
  );
  expect(show('=PRICEDISC(DATE(2008,3,1),DATE(2008,2,16),0.0525,100,2)')).toBe('#NUM!');
  expect(show('=YIELDDISC(DATE(2008,2,16),DATE(2008,3,1),0,100,2)')).toBe('#NUM!'); // precio 0
});

test('ISPMT no es IPMT: amortización constante, no cuota constante', () => {
  // Ejemplo documentado: 10 % anual, primer mes de un préstamo a 3 años de
  // 8.000.000 → −64.814,81. Es 8.000.000·(0,1/12)·(1/36 − 1).
  expect(num('=ISPMT(0.1/12,1,36,8000000)')).toBeCloseTo(-64814.8148148, 6);
  // El interés baja LINEALMENTE hasta 0 en el último periodo, porque el saldo
  // baja a partes iguales. Con IPMT (cuota constante) no llegaría nunca a 0.
  expect(num('=ISPMT(0.1/12,36,36,8000000)')).toBeCloseTo(0, 9);
  expect(num('=ISPMT(0.1/12,18,36,8000000)')).toBeCloseTo(8000000 * (0.1 / 12) * (18 / 36 - 1), 9);
  expect(num('=ISPMT(0.1/12,36,36,8000000)')).not.toBeCloseTo(
    num('=IPMT(0.1/12,36,36,8000000)'),
    3,
  );
  expect(show('=ISPMT(0.1,1,0,1000)')).toBe('#DIV/0!');
});
