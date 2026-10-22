/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de validación de datos (lista de valores):
 * troceo del texto del usuario, saneado desde datos no confiables, y cotejo
 * celda→validación (la última definida gana) + valor permitido.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  compileValidations,
  isAllowedValue,
  parseEsDate,
  parseEsNumberStrict,
  parseValidationValues,
  sanitizeDataValidation,
  validateValue,
  validationAt,
  validationLabel,
  validationMessage,
} from '../src/editor/sheets/format';
import { docsT } from '../src/i18n';
import type { DataValidation } from '../src/editor/types';

test('parseValidationValues: trocea por línea y coma, recorta, quita vacíos y duplica', () => {
  expect(parseValidationValues('Sí\nNo\nQuizá')).toEqual(['Sí', 'No', 'Quizá']);
  expect(parseValidationValues('a, b ,c')).toEqual(['a', 'b', 'c']);
  expect(parseValidationValues('  x \n\n x , y ,')).toEqual(['x', 'y']); // dedup + vacíos fuera
  expect(parseValidationValues('   ')).toEqual([]);
});

test('sanitizeDataValidation: exige rango válido y al menos un valor', () => {
  expect(sanitizeDataValidation({ range: 'A1:A10', values: ['Sí', 'No'] })).toEqual({
    range: 'A1:A10',
    values: ['Sí', 'No'],
  });
  // strict solo cuando es exactamente true.
  expect(sanitizeDataValidation({ range: 'B2', values: ['x'], strict: true })?.strict).toBe(true);
  expect(sanitizeDataValidation({ range: 'B2', values: ['x'], strict: 1 })?.strict).toBeUndefined();
  // Basura → null.
  expect(sanitizeDataValidation({ range: 'nope', values: ['x'] })).toBeNull();
  expect(sanitizeDataValidation({ range: 'A1', values: [] })).toBeNull();
  expect(sanitizeDataValidation({ range: 'A1', values: '  ' })).toBeNull();
  expect(sanitizeDataValidation(null)).toBeNull();
  // Valores no-cadena se descartan; vacíos y duplicados se limpian.
  expect(sanitizeDataValidation({ range: 'A1', values: ['x', 2, '', 'x', ' y '] })).toEqual({
    range: 'A1',
    values: ['x', 'y'],
  });
});

test('validationAt: la ÚLTIMA validación que cubre la celda gana; fuera → null', () => {
  const list: DataValidation[] = [
    { range: 'A1:C3', values: ['viejo'] },
    { range: 'B2', values: ['nuevo'] }, // solapa A1:C3 en B2 y, por ser posterior, manda
  ];
  const compiled = compileValidations(list);
  expect(validationAt(compiled, 1, 1)?.values).toEqual(['nuevo']); // B2 (col1,row1)
  expect(validationAt(compiled, 0, 0)?.values).toEqual(['viejo']); // A1 solo lo cubre la 1.ª
  expect(validationAt(compiled, 9, 9)).toBeNull(); // fuera de todo rango
});

test('isAllowedValue: vacío siempre permitido; cotejo exacto de la lista', () => {
  const v: DataValidation = { range: 'A1', values: ['Sí', 'No'] };
  expect(isAllowedValue(v, '')).toBe(true); // borrar la celda
  expect(isAllowedValue(v, '  ')).toBe(true); // solo espacios = vacío
  expect(isAllowedValue(v, ' Sí ')).toBe(true); // se recorta antes de cotejar
  expect(isAllowedValue(v, 'sí')).toBe(false); // sensible a mayúsculas (ponytail)
  expect(isAllowedValue(v, 'Tal vez')).toBe(false);
});

test('parseEsDate: dd/mm/aaaa, ISO y serial suelto → nº de serie 1900; basura → null', () => {
  expect(parseEsDate('1/1/1900')).toBe(1); // serial 1 = 1900-01-01
  expect(parseEsDate('28/2/1900')).toBe(59);
  expect(parseEsDate('1/3/1900')).toBe(61); // salta el día fantasma 60 (1900-02-29)
  expect(parseEsDate('2020-06-15')).toBe(parseEsDate('15/6/2020')); // ISO == es-ES
  expect(parseEsDate('45000')).toBe(45000); // un número suelto ya es su serial
  expect(parseEsDate('31/2/2020')).toBeNull(); // 31 de febrero no existe
  expect(parseEsDate('17/13/2020')).toBeNull(); // mes 13
  expect(parseEsDate('1/1/1899')).toBeNull(); // fuera del sistema 1900
  expect(parseEsDate('hola')).toBeNull();
  expect(parseEsDate('')).toBeNull();
  // Orden coherente (útil para «entre»/«antes»/«después»).
  expect(parseEsDate('2/1/2020')! > parseEsDate('1/1/2020')!).toBe(true);
});

test('parseEsNumberStrict: acepta números es-ES; rechaza texto con letras/símbolos', () => {
  expect(parseEsNumberStrict('123')).toBe(123);
  expect(parseEsNumberStrict('-5')).toBe(-5);
  expect(parseEsNumberStrict('1.234,56')).toBe(1234.56); // miles «.» + decimal «,»
  expect(parseEsNumberStrict(' 42 ')).toBe(42); // espacios OK
  // A diferencia del extractor tolerante, NO se queda con los dígitos:
  expect(parseEsNumberStrict('abc123')).toBeNull();
  expect(parseEsNumberStrict('A1')).toBeNull();
  expect(parseEsNumberStrict('12 kg')).toBeNull();
  expect(parseEsNumberStrict('$50')).toBeNull();
  expect(parseEsNumberStrict('5abc')).toBeNull();
  expect(parseEsNumberStrict('')).toBeNull();
});

test('validateValue número: rechaza texto no numérico con dígitos (forma estricta)', () => {
  const ge: DataValidation = { range: 'A1', type: 'number', op: 'ge', a: '0' };
  expect(validateValue(ge, '123')).toBe(true); // número de verdad pasa
  expect(validateValue(ge, 'abc123')).toBe(false); // antes colaba como 123
  expect(validateValue(ge, 'A1')).toBe(false); // referencia, no número
  expect(validateValue(ge, '12 kg')).toBe(false); // unidad
  expect(validateValue(ge, '$50')).toBe(false); // moneda
});

test('sanitizeDataValidation: operando con letras/símbolos → null (no lo lee como número)', () => {
  expect(sanitizeDataValidation({ range: 'A1', type: 'number', op: 'ge', a: '5abc' })).toBeNull();
  expect(sanitizeDataValidation({ range: 'A1', type: 'textLength', op: 'ge', a: '3x' })).toBeNull();
  expect(
    sanitizeDataValidation({ range: 'A1', type: 'number', op: 'between', a: '1', b: '10kg' }),
  ).toBeNull();
});

test('validateValue número: entre/comparadores, entero y vacío', () => {
  const between: DataValidation = { range: 'A1', type: 'number', op: 'between', a: '1', b: '10' };
  expect(validateValue(between, '')).toBe(true); // vacío siempre OK (borrar)
  expect(validateValue(between, '5')).toBe(true);
  expect(validateValue(between, '10')).toBe(true); // inclusivo
  expect(validateValue(between, '0')).toBe(false);
  expect(validateValue(between, 'x')).toBe(false); // no numérico
  const ge: DataValidation = { range: 'A1', type: 'number', op: 'ge', a: '0', integer: true };
  expect(validateValue(ge, '3')).toBe(true);
  expect(validateValue(ge, '2,5')).toBe(false); // decimal con «integer»
  expect(validateValue(ge, '-1')).toBe(false);
  const ne: DataValidation = { range: 'A1', type: 'number', op: 'notBetween', a: '1', b: '10' };
  expect(validateValue(ne, '11')).toBe(true);
  expect(validateValue(ne, '5')).toBe(false);
});

test('validateValue fecha: entre/después parsea la fecha del candidato', () => {
  const between: DataValidation = {
    range: 'A1',
    type: 'date',
    op: 'between',
    a: '1/1/2020',
    b: '31/12/2020',
  };
  expect(validateValue(between, '15/6/2020')).toBe(true);
  expect(validateValue(between, '2020-06-15')).toBe(true); // ISO también
  expect(validateValue(between, '1/1/2019')).toBe(false);
  expect(validateValue(between, 'no es fecha')).toBe(false);
  const after: DataValidation = { range: 'A1', type: 'date', op: 'gt', a: '1/1/2020' };
  expect(validateValue(after, '2/1/2020')).toBe(true);
  expect(validateValue(after, '31/12/2019')).toBe(false);
});

test('validateValue longitud del texto', () => {
  const le: DataValidation = { range: 'A1', type: 'textLength', op: 'le', a: '5' };
  expect(validateValue(le, 'hola')).toBe(true); // 4
  expect(validateValue(le, 'abcdef')).toBe(false); // 6
  expect(validateValue(le, '')).toBe(true);
  const bet: DataValidation = { range: 'A1', type: 'textLength', op: 'between', a: '2', b: '4' };
  expect(validateValue(bet, 'abc')).toBe(true);
  expect(validateValue(bet, 'a')).toBe(false);
});

test('validateValue fórmula: delega en el evaluador inyectado; sin él, válida', () => {
  const v: DataValidation = { range: 'A1', type: 'formula', formula: '=A1>0' };
  expect(validateValue(v, '5', (f) => f === '=A1>0')).toBe(true);
  expect(validateValue(v, '5', () => false)).toBe(false);
  expect(validateValue(v, '5')).toBe(true); // sin evaluador no se puede comprobar → válida
  expect(validateValue(v, '')).toBe(true); // vacío siempre OK
});

test('sanitizeDataValidation: criterios número/fecha/longitud/fórmula + mensaje', () => {
  // número «entre» necesita a y b; el tipo 'list' queda implícito.
  expect(
    sanitizeDataValidation({ range: 'A1', type: 'number', op: 'between', a: '1', b: '5' }),
  ).toEqual({ range: 'A1', type: 'number', op: 'between', a: '1', b: '5' });
  expect(sanitizeDataValidation({ range: 'A1', type: 'number', op: 'between', a: '1' })).toBeNull(); // falta b
  expect(
    sanitizeDataValidation({ range: 'A1', type: 'number', op: 'le', a: '10', integer: true }),
  ).toEqual({ range: 'A1', type: 'number', op: 'le', a: '10', integer: true });
  expect(sanitizeDataValidation({ range: 'A1', type: 'number', op: 'nope', a: '1' })).toBeNull();
  expect(sanitizeDataValidation({ range: 'A1', type: 'date', op: 'gt', a: '1/1/2020' })).toEqual({
    range: 'A1',
    type: 'date',
    op: 'gt',
    a: '1/1/2020',
  });
  expect(sanitizeDataValidation({ range: 'A1', type: 'date', op: 'gt', a: 'nope' })).toBeNull();
  expect(sanitizeDataValidation({ range: 'A1', type: 'textLength', op: 'ge', a: '3' })).toEqual({
    range: 'A1',
    type: 'textLength',
    op: 'ge',
    a: '3',
  });
  expect(sanitizeDataValidation({ range: 'A1', type: 'formula', formula: '=A1>0' })).toEqual({
    range: 'A1',
    type: 'formula',
    formula: '=A1>0',
  });
  expect(sanitizeDataValidation({ range: 'A1', type: 'formula', formula: '  ' })).toBeNull();
  // mensaje de error recortado + strict.
  expect(
    sanitizeDataValidation({
      range: 'A1',
      type: 'number',
      op: 'ge',
      a: '0',
      error: '  malo ',
      strict: true,
    }),
  ).toEqual({ range: 'A1', type: 'number', op: 'ge', a: '0', error: 'malo', strict: true });
  // 'list' explícito: type se omite (retrocompat).
  expect(
    sanitizeDataValidation({ range: 'A1', type: 'list', values: ['a'] })?.type,
  ).toBeUndefined();
});

test('validationMessage y validationLabel por criterio', () => {
  const es = docsT('es');
  expect(validationMessage({ range: 'A1', values: ['Sí', 'No'] }, 'B2')).toContain('Sí, No');
  expect(validationMessage({ range: 'A1', type: 'number', op: 'ge', a: '0' }, 'B2')).toContain(
    'B2',
  );
  expect(
    validationMessage({ range: 'A1', type: 'number', op: 'ge', a: '0', error: 'usa un nº' }, 'B2'),
  ).toBe('usa un nº'); // el mensaje del usuario manda
  expect(validationLabel({ range: 'A1', values: ['a', 'b'] }, es)).toBe('a, b');
  expect(validationLabel({ range: 'A1', type: 'number', op: 'between', a: '1', b: '5' }, es)).toBe(
    'Número entre 1 y 5',
  );
  expect(validationLabel({ range: 'A1', type: 'date', op: 'gt', a: '1/1/2020' }, es)).toBe(
    'Fecha > 1/1/2020',
  );
  expect(validationLabel({ range: 'A1', type: 'textLength', op: 'le', a: '5' }, es)).toBe(
    'Longitud del texto ≤ 5',
  );
  expect(validationLabel({ range: 'A1', type: 'formula', formula: '=A1>0' }, es)).toBe(
    'Fórmula: =A1>0',
  );
});
