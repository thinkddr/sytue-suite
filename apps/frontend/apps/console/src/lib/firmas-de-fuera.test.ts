/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import { organizacionesDe } from './firmas-de-fuera';

describe('organizaciones de confianza', () => {
  test('una por línea o separadas, en minúsculas y sin repetir', () => {
    expect(organizacionesDe(' Acme\nglobex, acme;  initech \n\n')).toEqual([
      'acme',
      'globex',
      'initech',
    ]);
    expect(organizacionesDe('')).toEqual([]);
  });
});
