/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test EXHAUSTIVO del formateador de máscaras personalizadas
 * (sheets/numfmt-mask.ts): secciones por signo, marcadores 0/#/?/,/./%,
 * literales, moneda, [Color], y máscaras de fecha/hora (mes vs minuto).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import { formatWithMask, splitSections } from '../src/editor/sheets/numfmt-mask';

const T = (mask: string, n: number): string => formatWithMask(mask, n).text;

test('troceado en secciones: respeta comillas y corchetes', () => {
  expect(splitSections('0.00;-0.00')).toEqual(['0.00', '-0.00']);
  expect(splitSections('0.00;"a;b";0')).toEqual(['0.00', '"a;b"', '0']);
  expect(splitSections('[Red];[Blue]')).toEqual(['[Red]', '[Blue]']);
  expect(splitSections('0')).toEqual(['0']);
});

test('enteros, decimales y millares (salida es-ES)', () => {
  expect(T('0.00', 1234.5)).toBe('1234,50');
  expect(T('#,##0.00', 1234.5)).toBe('1.234,50');
  expect(T('#,##0', 1234.5)).toBe('1.235'); // redondeo
  expect(T('#,##0', 1234.4)).toBe('1.234');
  expect(T('0', 0)).toBe('0');
  expect(T('#', 0)).toBe(''); // # no fuerza el cero
  expect(T('00000', 42)).toBe('00042'); // relleno de ceros
  expect(T('00', 123)).toBe('123'); // desbordamiento visible
});

test('decimales: # recorta ceros finales, 0 los conserva', () => {
  expect(T('#.##', 5)).toBe('5'); // ambos # → sin decimales
  expect(T('#.##', 5.5)).toBe('5,5');
  expect(T('0.##', 5.5)).toBe('5,5');
  expect(T('0.00', 5.5)).toBe('5,50');
  expect(T('.00', 12.5)).toBe('12,50'); // sin placeholder entero
});

test('máscara con >100 decimales: acota a 100 sin RangeError de toFixed', () => {
  const mask = '0.' + '#'.repeat(101); // 101 marcadores decimales, alcanzable desde el input
  expect(() => T(mask, 3.14)).not.toThrow(); // antes: crash en toFixed (0..100)
  expect(T(mask, 3.14).startsWith('3,14')).toBe(true);
  const forced = T('0.' + '0'.repeat(101), 0.5); // «0» fuerza los decimales
  expect(forced.startsWith('0,5')).toBe(true);
  expect(forced.length - forced.indexOf(',') - 1).toBe(100); // topado a 100, no 101
});

test('negativos: signo automático, sección dedicada y paréntesis', () => {
  expect(T('#,##0.00', -1234.5)).toBe('-1.234,50'); // 1 sección → «-» automático
  expect(T('0.00;0.00', -5)).toBe('5,00'); // sección negativa sin «-» → magnitud
  expect(T('#,##0.00;(#,##0.00)', -1234.5)).toBe('(1.234,50)');
  expect(T('#,##0;-#,##0', -1234)).toBe('-1.234');
});

test('cero: tercera sección y ocultado', () => {
  expect(T('0.00;-0.00;"cero"', 0)).toBe('cero');
  expect(T('0.00;-0.00;', 0)).toBe(''); // sección de cero vacía → oculto
  expect(T('0;(0)', 0)).toBe('0'); // sin sección de cero → usa la positiva
});

test('porcentaje y escalado por millares', () => {
  expect(T('0%', 0.5)).toBe('50%');
  expect(T('0.0%', 0.1234)).toBe('12,3%');
  expect(T('#,##0,', 1234567)).toBe('1.235'); // coma final: /1000 + agrupa
  expect(T('0,,', 12345678)).toBe('12'); // dos comas finales: /1e6
});

test('moneda: símbolo prefijo, sufijo y [$€-xxx]', () => {
  expect(T('$#,##0.00', 1234.5)).toBe('$1.234,50');
  expect(T('#,##0.00 €', 1234.5)).toBe('1.234,50 €');
  expect(T('[$€-C0A]#,##0.00', 1234.5)).toBe('€1.234,50');
  expect(T('"USD "#,##0.00', 1234.5)).toBe('USD 1.234,50'); // literal entre comillas
  expect(T('\\$0', 5)).toBe('$5'); // escape
});

test('color: [Red]/[Rojo] y por-sección', () => {
  expect(formatWithMask('[Red]0.00', 5)).toEqual({ text: '5,00', color: '#DC2626' });
  expect(formatWithMask('0.00;[Rojo]-0.00', -5)).toEqual({ text: '-5,00', color: '#DC2626' });
  expect(formatWithMask('[Blue]0', 5)).toEqual({ text: '5', color: '#2563EB' });
  expect(formatWithMask('0.00', 5)).toEqual({ text: '5,00' }); // sin color
});

test('General y no finitos', () => {
  expect(T('General', 1234.5)).toBe('1234.5'); // defiere al motor (no localiza)
  expect(T('General', -5)).toBe('-5');
  expect(T('0.00', Infinity)).toBe(formatWithMask('', Infinity).text);
  expect(Number.isNaN(NaN) && T('0.00', NaN)).toBeTruthy();
});

test('fecha: dd/mm/yyyy y variantes con nombre', () => {
  expect(T('dd/mm/yyyy', 45292)).toBe('01/01/2024'); // 2024-01-01
  expect(T('d/m/yyyy', 45292)).toBe('1/1/2024');
  expect(T('dd/mmm/yyyy', 45292)).toBe('01/ene/2024');
  expect(T('mmmm yyyy', 45292)).toBe('enero 2024');
  expect(T('yyyy-mm-dd', 45658)).toBe('2025-01-01'); // mm=mes (vecinos yyyy/dd)
  expect(T('dddd', 45292)).toBe('lunes'); // 2024-01-01 es lunes
  expect(T('ddd d "de" mmmm', 45292)).toBe('lun 1 de enero');
});

test('hora: mm como minuto junto a h/s, 24 h y AM/PM', () => {
  expect(T('hh:mm:ss', 0.5)).toBe('12:00:00'); // mediodía
  expect(T('hh:mm', 0.75)).toBe('18:00');
  expect(T('h:mm', 0.25)).toBe('6:00');
  expect(T('h:mm AM/PM', 0.75)).toBe('6:00 PM');
  expect(T('h:mm am/pm', 0.5)).toBe('12:00 pm');
  expect(T('dd/mm/yyyy hh:mm', 45292.5)).toBe('01/01/2024 12:00'); // fecha + hora
});

test('fecha fuera de rango → General', () => {
  expect(T('dd/mm/yyyy', -3)).toBe('-3');
  expect(T('dd/mm/yyyy', 60)).toBe('29/02/1900'); // día fantasma del bug 1900
});

/* ---------- Ola: tiempo transcurrido, condiciones y literales entre dígitos ---------- */

test('tiempo transcurrido [h]/[mm]/[ss]: acumula, no da la vuelta', () => {
  const H = 1 / 24;
  expect(T('[h]:mm:ss', 26.5 * H)).toBe('26:30:00'); // 26 h ≠ 2 h (h normal daría la vuelta)
  expect(T('[h]:mm', 1.5 * H)).toBe('1:30');
  expect(T('[hh]:mm', 1.5 * H)).toBe('01:30'); // [hh] rellena a 2
  expect(T('[m]', 90 * 60 * (1 / 86400))).toBe('90'); // minutos totales
  expect(T('[mm]:ss', 90.5 * 60 * (1 / 86400))).toBe('90:30');
  expect(T('[s]', 3661 / 86400)).toBe('3661'); // segundos totales
  expect(T('[h]:mm', -1.5 * H)).toBe('-1:30'); // negativo con signo (Excel da ###)
  expect(T('[h] "h"', 2 * H)).toBe('2 h'); // literal entre comillas
});

test('condiciones de sección [>=n]: gana la primera que casa; sin condición = reserva', () => {
  const M = '[>=100]"alto";[<0]"neg";0.00';
  expect(T(M, 150)).toBe('alto');
  expect(T(M, -5)).toBe('-neg'); // el signo se conserva (no hay «sección de negativos»)
  expect(T(M, 42)).toBe('42,00'); // reserva
  expect(T('[>5]0;0', 3)).toBe('3');
  expect(T('[<>0]0;"cero"', 0)).toBe('cero');
  expect(T('[>100]0', 5)).toBe('5'); // ninguna casa y no hay reserva → General
});

test('literales ENTRE placeholders de dígito entero (se reparten por la derecha)', () => {
  expect(T('00000-0000', 123456789)).toBe('12345-6789'); // ZIP+4
  expect(T('00-00', 1234)).toBe('12-34');
  expect(T('00-00', 123)).toBe('01-23'); // relleno de ceros
  expect(T('000-000-000', 123456789)).toBe('123-456-789');
  expect(T('00', 123)).toBe('123'); // sobrante al 1.er placeholder (sin regresión)
  expect(T('#,##0.00', 1234.5)).toBe('1.234,50'); // con agrupado NO se reparte
});
