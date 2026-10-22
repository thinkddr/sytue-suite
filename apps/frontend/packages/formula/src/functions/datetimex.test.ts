// Fecha añadida: DAYS360 (US/europeo) y las variantes .INTL de NETWORKDAYS y
// WORKDAY con fin de semana configurable. Valores contrastados con Excel. `bun test`.
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

test('DAYS360: método US (NASD) por defecto', () => {
  expect(calc('=DAYS360(DATE(2020,1,1), DATE(2021,1,1))')).toBe(360);
  expect(calc('=DAYS360(DATE(2020,1,1), DATE(2020,2,1))')).toBe(30);
  expect(calc('=DAYS360(DATE(2020,1,30), DATE(2020,3,30))')).toBe(60);
  expect(calc('=DAYS360(DATE(2021,1,1), DATE(2020,1,1))')).toBe(-360); // no simétrica
});

test('DAYS360: US vs europeo difieren en fin de mes', () => {
  // 29-feb → 31-mar: US cuenta 30 (fin de feb→30, fin de mes destino→30); EU 31.
  expect(calc('=DAYS360(DATE(2020,2,29), DATE(2020,3,31))')).toBe(30);
  expect(calc('=DAYS360(DATE(2020,2,29), DATE(2020,3,31), TRUE)')).toBe(31);
});

test('NETWORKDAYS.INTL: fin de semana por defecto y configurable', () => {
  expect(calc('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,8))')).toBe(5); // lun-vie
  expect(calc('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,10))')).toBe(5); // + fin de semana
  expect(calc('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,10), 11)')).toBe(6); // solo domingo libre
  expect(calc('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,10), "1000000")')).toBe(6); // solo lunes libre
  expect(calc('=NETWORKDAYS.INTL(DATE(2021,1,8), DATE(2021,1,4))')).toBe(-5); // invertido
});

test('NETWORKDAYS.INTL: descuenta festivos entre semana', () => {
  expect(calc('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,8), 1, DATE(2021,1,6))')).toBe(4);
});

test('NETWORKDAYS.INTL: fin de semana inválido', () => {
  expect(show('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,8), 18)')).toBe('#NUM!');
  expect(show('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,8), "111")')).toBe('#VALUE!');
  expect(show('=NETWORKDAYS.INTL(DATE(2021,1,4), DATE(2021,1,8), "1111111")')).toBe('#VALUE!');
});

test('WORKDAY.INTL: avanza días laborables', () => {
  expect(calc('=WORKDAY.INTL(DATE(2021,1,1), 1)')).toBe(calc('=DATE(2021,1,4)')); // vie +1 → lun
  expect(calc('=WORKDAY.INTL(DATE(2021,1,1), 5)')).toBe(calc('=DATE(2021,1,8)'));
  expect(calc('=WORKDAY.INTL(DATE(2021,1,4), 0)')).toBe(calc('=DATE(2021,1,4)')); // 0 días
  expect(calc('=WORKDAY.INTL(DATE(2021,1,1), 1, 11)')).toBe(calc('=DATE(2021,1,2)')); // sáb es laborable
  expect(calc('=WORKDAY.INTL(DATE(2021,1,8), -1)')).toBe(calc('=DATE(2021,1,7)')); // hacia atrás
});

test('WORKDAY.INTL: salta festivos', () => {
  expect(calc('=WORKDAY.INTL(DATE(2021,1,1), 1, 1, DATE(2021,1,4))')).toBe(calc('=DATE(2021,1,5)'));
});
