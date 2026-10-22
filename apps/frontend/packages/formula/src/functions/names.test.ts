// Rangos con nombre (Engine.defineName/deleteName) e ISFORMULA. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function setCol(e: Engine, letter: string, vals: string[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, v));
}

test('nombre = constante: se resuelve en fórmulas', () => {
  const e = new Engine();
  expect(e.defineName('TAX', '0.21')).toBe(true);
  e.setCell('Hoja1', 'A1', '=100*TAX');
  expect(e.getValue('Hoja1', 'A1')).toBe(21);
});

test('nombre = rango: agrega y sigue los cambios de las celdas', () => {
  const e = new Engine();
  setCol(e, 'A', ['10', '20', '30']);
  e.setCell('Hoja1', 'C1', '=SUM(Ventas)'); // aún no existe → #NAME?
  expect(e.getDisplay('Hoja1', 'C1')).toBe('#NAME?');
  e.defineName('Ventas', 'A1:A3');
  expect(e.getValue('Hoja1', 'C1')).toBe(60); // ya resuelve
  e.setCell('Hoja1', 'A2', '25'); // cambia una celda del rango con nombre
  expect(e.getValue('Hoja1', 'C1')).toBe(65); // el dependiente se invalida
});

test('nombre = fórmula, anclada en su hoja base', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '5');
  e.defineName('Doble', 'A1*2');
  e.setCell('Hoja1', 'Z9', '=Doble');
  expect(e.getValue('Hoja1', 'Z9')).toBe(10); // A1 se ancla en Hoja1!A1
});

test('un nombre de rango a nivel de celda DERRAMA', () => {
  const e = new Engine();
  setCol(e, 'A', ['7', '8', '9']);
  e.defineName('Col', 'A1:A3');
  e.setCell('Hoja1', 'C1', '=Col');
  expect(e.getValue('Hoja1', 'C1')).toBe(7);
  expect(e.getValue('Hoja1', 'C2')).toBe(8);
  expect(e.getValue('Hoja1', 'C3')).toBe(9);
});

test('deleteName vuelve a dejar el nombre sin resolver', () => {
  const e = new Engine();
  e.defineName('TAX', '0.21');
  e.setCell('Hoja1', 'A1', '=TAX');
  expect(e.getValue('Hoja1', 'A1')).toBe(0.21);
  expect(e.deleteName('TAX')).toBe(true);
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#NAME?');
});

test('nombres inválidos y listado', () => {
  const e = new Engine();
  expect(e.defineName('A1', '1')).toBe(false); // parece una referencia
  expect(e.defineName('con espacio', '1')).toBe(false);
  expect(e.defineName('mal(', '1')).toBe(false);
  expect(e.defineName('Rango', 'A1:::B2')).toBe(false); // la definición no parsea
  e.defineName('Uno', '1');
  e.defineName('Dos', '2');
  expect(e.definedNameList()).toEqual(['DOS', 'UNO']);
});

test('nombre recursivo no cuelga → #CIRC!', () => {
  const e = new Engine();
  e.defineName('Bucle', 'Bucle+1');
  e.setCell('Hoja1', 'A1', '=Bucle');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#CIRC!');
});

test('ISFORMULA distingue fórmula de literal; exige una referencia', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=1+1');
  e.setCell('Hoja1', 'A2', '5');
  e.setCell('Hoja1', 'B1', '=ISFORMULA(A1)');
  e.setCell('Hoja1', 'B2', '=ISFORMULA(A2)');
  e.setCell('Hoja1', 'B3', '=ISFORMULA(5)');
  expect(e.getValue('Hoja1', 'B1')).toBe(true);
  expect(e.getValue('Hoja1', 'B2')).toBe(false);
  expect(e.getDisplay('Hoja1', 'B3')).toBe('#VALUE!');
});

test('nombre de HOJA: solo existe en su hoja y allí tapa al del libro', () => {
  const e = new Engine({ sheets: ['Hoja1', 'Hoja2'] });
  e.defineName('Tasa', '0.21');
  expect(e.defineName('Tasa', '0.1', 'Hoja2', true)).toBe(true);
  e.setCell('Hoja1', 'A1', '=Tasa');
  e.setCell('Hoja2', 'A1', '=Tasa');
  expect(e.getValue('Hoja1', 'A1')).toBe(0.21);
  expect(e.getValue('Hoja2', 'A1')).toBe(0.1);
  expect(e.definedNameList()).toEqual(['TASA']);
  // Sin el del libro, desde otra hoja no se ve.
  e.deleteName('Tasa');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#NAME?');
  expect(e.getValue('Hoja2', 'A1')).toBe(0.1);
  expect(e.deleteName('Tasa', 'Hoja2')).toBe(true);
  expect(e.getDisplay('Hoja2', 'A1')).toBe('#NAME?');
});

test('un nombre de hoja sigue los cambios de su rango', () => {
  const e = new Engine({ sheets: ['Hoja1', 'Hoja2'] });
  e.setCell('Hoja2', 'B1', '4');
  e.defineName('Dato', 'B1', 'Hoja2', true);
  e.setCell('Hoja2', 'C1', '=Dato*2');
  expect(e.getValue('Hoja2', 'C1')).toBe(8);
  e.setCell('Hoja2', 'B1', '5');
  expect(e.getValue('Hoja2', 'C1')).toBe(10);
});
