/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La regla de un grupo dinámico se enseña en una frase llana (repaso del asistente
 * de alta). Comprueba Y/O, las listas de `in`, cada operador y que las condiciones
 * sin valor no cuentan. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { useT as traductor } from '../i18n/index.js';
import { describeRule, type BuilderRule } from './rules.js';

const es = traductor('es');
const en = traductor('en');

test('«todas» une con «y» y «alguna» con «o»', () => {
  const todas: BuilderRule = {
    match: 'all',
    conditions: [
      { attribute: 'department', op: 'eq', value: ' Ventas ' },
      { attribute: 'title', op: 'contains', value: 'jefe' },
    ],
  };
  expect(describeRule(todas, es, 'es')).toBe(
    'Entran las personas si su departamento es «Ventas» y su puesto contiene «jefe».',
  );
  expect(describeRule({ ...todas, match: 'any' }, es, 'es')).toBe(
    'Entran las personas si su departamento es «Ventas» o su puesto contiene «jefe».',
  );
});

test('`in` enumera los valores como alternativas, sin vacíos', () => {
  const r: BuilderRule = {
    match: 'all',
    conditions: [{ attribute: 'location', op: 'in', value: 'Madrid, Sevilla,, Bilbao ' }],
  };
  expect(describeRule(r, es, 'es')).toBe(
    'Entran las personas si su ubicación es «Madrid», «Sevilla» o «Bilbao».',
  );
  expect(describeRule(r, en, 'en')).toBe(
    'People join if their location is “Madrid”, “Sevilla” or “Bilbao”.',
  );
});

test('cada operador tiene su verbo', () => {
  const una = (op: BuilderRule['conditions'][number]['op']) =>
    describeRule(
      { match: 'all', conditions: [{ attribute: 'email', op, value: 'ana' }] },
      en,
      'en',
    );
  expect(una('eq')).toBe('People join if their email is “ana”.');
  expect(una('ne')).toBe('People join if their email is not “ana”.');
  expect(una('contains')).toBe('People join if their email contains “ana”.');
  expect(una('starts_with')).toBe('People join if their email starts with “ana”.');
});

test('las condiciones a medio escribir no entran en la frase', () => {
  const r: BuilderRule = {
    match: 'all',
    conditions: [
      { attribute: 'name', op: 'eq', value: '  ' },
      { attribute: 'department', op: 'ne', value: 'IT' },
    ],
  };
  expect(describeRule(r, es, 'es')).toBe('Entran las personas si su departamento no es «IT».');
  expect(describeRule({ match: 'any', conditions: [r.conditions[0]!] }, es, 'es')).toBe(
    es('groups.sentence.empty'),
  );
});
