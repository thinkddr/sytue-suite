// Funciones de base de datos (DSUM/DCOUNT/DAVERAGE/DMAX/DMIN/DGET/…). Valores
// contrastados con una hoja real. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

/** Base con cabeceras Nombre/Depto/Ventas + zonas de criterios. */
function db(): Engine {
  const e = new Engine();
  const rows = [
    ['Nombre', 'Depto', 'Ventas'],
    ['Ana', 'Norte', '100'],
    ['Luis', 'Sur', '200'],
    ['Eva', 'Norte', '300'],
    ['Juan', 'Sur', '400'],
  ];
  rows.forEach((r, i) =>
    r.forEach((v, c) => e.setCell('Hoja1', `${String.fromCharCode(65 + c)}${i + 1}`, v)),
  );
  // Criterio 1: Depto = Norte  (E1:E2)
  e.setCell('Hoja1', 'E1', 'Depto');
  e.setCell('Hoja1', 'E2', 'Norte');
  // Criterio 2: Ventas > 350   (G1:G2)
  e.setCell('Hoja1', 'G1', 'Ventas');
  e.setCell('Hoja1', 'G2', '>350');
  return e;
}

test('DSUM / DAVERAGE / DMAX / DMIN por nombre de campo y por índice', () => {
  const e = db();
  e.setCell('Hoja1', 'Z1', '=DSUM(A1:C5, "Ventas", E1:E2)'); // Norte: 100+300
  e.setCell('Hoja1', 'Z2', '=DSUM(A1:C5, 3, E1:E2)'); // campo por índice
  e.setCell('Hoja1', 'Z3', '=DAVERAGE(A1:C5, "Ventas", E1:E2)');
  e.setCell('Hoja1', 'Z4', '=DMAX(A1:C5, "Ventas", E1:E2)');
  e.setCell('Hoja1', 'Z5', '=DMIN(A1:C5, "Ventas", E1:E2)');
  expect(e.getValue('Hoja1', 'Z1')).toBe(400);
  expect(e.getValue('Hoja1', 'Z2')).toBe(400);
  expect(e.getValue('Hoja1', 'Z3')).toBe(200);
  expect(e.getValue('Hoja1', 'Z4')).toBe(300);
  expect(e.getValue('Hoja1', 'Z5')).toBe(100);
});

test('DCOUNT (números) y DCOUNTA (no vacíos)', () => {
  const e = db();
  e.setCell('Hoja1', 'Z1', '=DCOUNT(A1:C5, "Ventas", E1:E2)'); // 2 registros Norte
  e.setCell('Hoja1', 'Z2', '=DCOUNTA(A1:C5, "Nombre", E1:E2)'); // 2 nombres (texto)
  expect(e.getValue('Hoja1', 'Z1')).toBe(2);
  expect(e.getValue('Hoja1', 'Z2')).toBe(2);
});

test('DGET: un único registro; 0 → #VALUE!, >1 → #NUM!', () => {
  const e = db();
  e.setCell('Hoja1', 'Z1', '=DGET(A1:C5, "Nombre", G1:G2)'); // Ventas>350 → solo Juan
  expect(e.getValue('Hoja1', 'Z1')).toBe('Juan');
  e.setCell('Hoja1', 'Z2', '=DGET(A1:C5, "Nombre", E1:E2)'); // Norte → 2 → #NUM!
  expect(e.getDisplay('Hoja1', 'Z2')).toBe('#NUM!');
  // criterio que no casa nadie
  e.setCell('Hoja1', 'G2', '>9999');
  e.setCell('Hoja1', 'Z3', '=DGET(A1:C5, "Nombre", G1:G2)');
  expect(e.getDisplay('Hoja1', 'Z3')).toBe('#VALUE!');
});

test('criterio numérico (Ventas > 350) suma solo lo que casa', () => {
  const e = db();
  e.setCell('Hoja1', 'Z1', '=DSUM(A1:C5, "Ventas", G1:G2)');
  expect(e.getValue('Hoja1', 'Z1')).toBe(400); // solo Juan
});

test('varias filas de criterio = O lógico', () => {
  const e = db();
  // E1:E3 → Depto = Norte O Depto = Sur (todas las filas)
  e.setCell('Hoja1', 'E3', 'Sur');
  e.setCell('Hoja1', 'Z1', '=DSUM(A1:C5, "Ventas", E1:E3)');
  expect(e.getValue('Hoja1', 'Z1')).toBe(1000); // 100+200+300+400
});

test('sigue los cambios de la base (dependencias)', () => {
  const e = db();
  e.setCell('Hoja1', 'Z1', '=DSUM(A1:C5, "Ventas", E1:E2)');
  expect(e.getValue('Hoja1', 'Z1')).toBe(400);
  e.setCell('Hoja1', 'C2', '150'); // Ana pasa de 100 a 150
  expect(e.getValue('Hoja1', 'Z1')).toBe(450);
});
