// Checks de los nombres definidos del libro (sheets/nombres.ts). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '@sytue/formula';
import type { DefinedName, SheetModel } from '../src/editor/types';
import {
  applyNameCommand,
  defineNames,
  moverNombres,
  nameError,
  namesVisibleFrom,
  normalizeRef,
  quitarHojaDeNombres,
  renombrarEnFormula,
  renombrarHojaEnNombres,
  sanitizeNames,
} from '../src/editor/sheets/nombres';

const book = (names: DefinedName[] = []) => ({
  names,
  sheets: ['Hoja1', 'Datos'],
  tables: ['Ventas'],
});

test('valida como Excel: identificador sin forma de referencia, único por ámbito', () => {
  const ok = (name: string, sheet?: string): DefinedName => ({
    name,
    ref: '1',
    ...(sheet ? { sheet } : {}),
  });
  expect(nameError(ok('Tasa_IVA.2026'), book())).toBeNull();
  expect(nameError(ok(''), book())).toBe('empty');
  for (const bad of [
    'A1',
    '$B$2',
    'XFD1',
    'R1C1',
    'rc',
    'R2',
    'C7',
    'con espacio',
    '1abc',
    'TRUE',
  ]) {
    expect(nameError(ok(bad), book())).toBe('invalid');
  }
  expect(nameError(ok('x'.repeat(256)), book())).toBe('tooLong');
  const tasa = ok('Tasa');
  expect(nameError(ok('TASA'), book([tasa]))).toBe('taken'); // sin distinguir mayúsculas
  expect(nameError(ok('Tasa', 'Datos'), book([tasa]))).toBeNull(); // otro ámbito
  expect(nameError(ok('tasa'), book([tasa]), tasa)).toBeNull(); // es el que se edita
  expect(nameError(ok('ventas'), book())).toBe('taken'); // se llama como una tabla
  expect(nameError(ok('Tasa', 'NoEsta'), book())).toBe('noSheet');
  expect(nameError({ name: 'Tasa', ref: '' }, book())).toBe('badRef');
  expect(nameError({ name: 'Tasa', ref: 'A1:::B2' }, book())).toBe('badRef');
});

test('un rango suelto se guarda cualificado y absoluto; lo demás, tal cual', () => {
  expect(normalizeRef('B2:B9', 'Hoja1')).toBe('Hoja1!$B$2:$B$9');
  expect(normalizeRef('=b2', 'Mi hoja')).toBe("'Mi hoja'!$B$2");
  expect(normalizeRef("'Mi hoja'!a1:c3", 'Hoja1')).toBe("'Mi hoja'!$A$1:$C$3");
  expect(normalizeRef('=0.21', 'Hoja1')).toBe('0.21');
  expect(normalizeRef('B2*2', 'Hoja1')).toBe('B2*2');
  expect(normalizeRef('A:A', 'Hoja1')).toBe('A:A');
});

test('renombrar toca el nombre y nada que se le parezca', () => {
  expect(renombrarEnFormula('=SUM(Ventas)+ventas*2+Ventas2', 'Ventas', 'Total')).toBe(
    '=SUM(Total)+Total*2+Ventas2',
  );
  expect(renombrarEnFormula('="Ventas"&Ventas', 'Ventas', 'T')).toBe('="Ventas"&T');
  expect(renombrarEnFormula('=Ventas!A1+Ventas(1)', 'Ventas', 'T')).toBe('=Ventas!A1+Ventas(1)');
  expect(renombrarEnFormula('=Tabla[Ventas]+Tabla[@Ventas]', 'Ventas', 'T')).toBe(
    '=Tabla[Ventas]+Tabla[@Ventas]',
  );
  expect(renombrarEnFormula('Ventas', 'Ventas', 'T')).toBe('Ventas'); // un literal no
});

const modelo = (): SheetModel => ({
  active: 0,
  sheets: [
    { name: 'Hoja1', cells: { A1: '5', B1: '=Tasa*A1', C1: '<b>=Tasa</b>' } },
    { name: 'Datos', cells: { A1: '=Tasa', B1: '=Doble' } },
  ],
  names: [
    { name: 'Tasa', ref: '0.21' },
    { name: 'Tasa', ref: '0.1', sheet: 'Datos' },
    { name: 'Doble', ref: 'Tasa*2' },
  ],
});

test('crear: valida con el libro entero y normaliza la referencia', () => {
  const res = applyNameCommand(
    modelo(),
    'nameset',
    JSON.stringify({ name: 'Rango', ref: 'A1:A3' }),
  );
  expect(res && 'model' in res ? res.model.names?.at(-1) : res).toEqual({
    name: 'Rango',
    ref: 'Hoja1!$A$1:$A$3',
  });
  expect(applyNameCommand(modelo(), 'nameset', JSON.stringify({ name: 'tasa', ref: '1' }))).toEqual(
    { error: 'taken' },
  );
  expect(applyNameCommand(modelo(), 'nameset', 'no es JSON')).toBeNull();
});

test('renombrar reescribe solo las fórmulas que veían ESE nombre', () => {
  const res = applyNameCommand(
    modelo(),
    'nameset',
    JSON.stringify({ name: 'IVA', ref: '0.21', prev: { name: 'Tasa' } }),
  );
  if (!res || !('model' in res)) throw new Error(JSON.stringify(res));
  const [h1, datos] = res.model.sheets;
  expect(h1!.cells).toEqual({ A1: '5', B1: '=IVA*A1', C1: '<b>=IVA</b>' }); // conserva el formato
  // En «Datos» manda la Tasa de la hoja: sus fórmulas no cambian.
  expect(datos!.cells).toEqual({ A1: '=Tasa', B1: '=Doble' });
  // Doble es del libro y se lee desde la primera hoja: veía la Tasa del libro.
  expect(res.model.names).toEqual([
    { name: 'IVA', ref: '0.21' },
    { name: 'Tasa', ref: '0.1', sheet: 'Datos' },
    { name: 'Doble', ref: 'IVA*2' },
  ]);
});

test('renombrar no reescribe una hoja protegida: se niega entero', () => {
  const m = modelo();
  m.sheets[0]!.protect = {};
  const res = applyNameCommand(
    m,
    'nameset',
    JSON.stringify({ name: 'IVA', ref: '0.21', prev: { name: 'Tasa' } }),
  );
  expect(res).toEqual({ error: 'protected' });
});

test('borrar quita el de ese ámbito y deja las fórmulas (pasarán a #NAME?)', () => {
  const res = applyNameCommand(
    modelo(),
    'namedel',
    JSON.stringify({ name: 'TASA', sheet: 'Datos' }),
  );
  if (!res || !('model' in res)) throw new Error(JSON.stringify(res));
  expect(res.model.names?.map((n) => `${n.sheet ?? ''}!${n.name}`)).toEqual(['!Tasa', '!Doble']);
  expect(res.model.sheets).toEqual(modelo().sheets);
  expect(applyNameCommand(modelo(), 'namedel', JSON.stringify({ name: 'Nada' }))).toBeNull();
});

test('el motor resuelve los de hoja antes que los del libro', () => {
  const m = modelo();
  const e = new Engine({ sheets: m.sheets.map((s) => s.name) });
  defineNames(e, [...m.names!, { name: 'Huérfano', ref: '1', sheet: 'NoEsta' }]);
  for (const s of m.sheets) for (const ref in s.cells) e.setCell(s.name, ref, s.cells[ref]!);
  expect(e.getValue('Hoja1', 'B1')).toBeCloseTo(1.05);
  expect(e.getValue('Datos', 'A1')).toBe(0.1);
  expect(e.getValue('Datos', 'B1')).toBe(0.42); // Doble se ancla en Hoja1: Tasa del libro
  expect(namesVisibleFrom(m.names, 'Datos')).toEqual(['Tasa', 'Doble']);
});

test('las definiciones siguen a sus celdas al insertar y borrar filas', () => {
  const names: DefinedName[] = [
    { name: 'A', ref: 'Hoja1!$A$1:$A$5' },
    { name: 'B', ref: 'Datos!$A$1' }, // otra hoja: no se mueve
    { name: 'C', ref: '$B$3*2' }, // sin hoja: la primera (Hoja1)
    { name: 'D', ref: '$A$3', sheet: 'Datos' }, // de hoja: se lee en «Datos»
  ];
  const ins = moverNombres(names, 'Hoja1', 'Hoja1', {
    eje: 'fila',
    desde: 0,
    cantidad: 1,
    tope: 100,
  });
  expect(ins?.map((n) => n.ref)).toEqual(['Hoja1!$A$2:$A$6', 'Datos!$A$1', '$B$4*2', '$A$3']);
  expect(ins?.[1]).toBe(names[1]); // lo que no cambia no se copia
  const del = moverNombres(names, 'Hoja1', 'Hoja1', {
    eje: 'fila',
    desde: 2,
    cantidad: -1,
    tope: 100,
  });
  expect(del?.map((n) => n.ref)).toEqual(['Hoja1!$A$1:$A$4', 'Datos!$A$1', '#¡REF!*2', '$A$3']);
});

test('renombrar o borrar una hoja arrastra sus nombres', () => {
  const names: DefinedName[] = [
    { name: 'A', ref: 'Datos!$A$1+Hoja1!B2' },
    { name: 'B', ref: '1', sheet: 'Datos' },
  ];
  expect(renombrarHojaEnNombres(names, 'Datos', 'Mis datos')).toEqual([
    { name: 'A', ref: "'Mis datos'!$A$1+Hoja1!B2" },
    { name: 'B', ref: '1', sheet: 'Mis datos' },
  ]);
  expect(quitarHojaDeNombres(names, 'Datos')).toEqual([names[0]!]);
  expect(quitarHojaDeNombres([names[1]!], 'Datos')).toBeUndefined();
});

test('el saneado conserva lo que tiene forma y tira la basura', () => {
  expect(
    sanitizeNames([
      { name: 'Año', ref: '2026' }, // con tilde: el motor no lo lee, pero no se pierde
      { name: 'X', ref: '1', sheet: '', comment: '' },
      { name: '', ref: '1' },
      { name: 'Y' },
      'basura',
    ]),
  ).toEqual([
    { name: 'Año', ref: '2026' },
    { name: 'X', ref: '1' },
  ]);
  expect(sanitizeNames([])).toBeUndefined();
  expect(sanitizeNames('no')).toBeUndefined();
});
