/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { eventoDe, instruccionEvento } from './ai.js';

test('la instrucción lleva la fecha de hoy y la zona', () => {
  const i = instruccionEvento(new Date('2026-09-10T08:00:00Z'), 'Europe/Madrid');
  expect(i).toContain('2026-09-10T08:00:00.000Z');
  expect(i).toContain('Europe/Madrid');
});

test('un evento completo sale entero', () => {
  const e = eventoDe(
    '{"titulo":"Comida con Ana","inicio":"2026-09-17T14:00:00+02:00","fin":"2026-09-17T15:30:00+02:00","lugar":"Casa Lucio"}',
  );
  expect(e?.titulo).toBe('Comida con Ana');
  expect(e?.lugar).toBe('Casa Lucio');
  // Normalizada a UTC: lo que sale de aquí se le pasa a `new Date` sin más.
  expect(e?.fin).toBe('2026-09-17T13:30:00.000Z');
});

// Sin título no hay propuesta: prerrellenar solo la hora obliga a adivinar de qué
// era la cita.
test('sin título no se propone nada', () => {
  expect(eventoDe('{"inicio":"2026-09-17T14:00:00+02:00"}')).toBeNull();
  expect(eventoDe('no he entendido nada')).toBeNull();
});

// El modelo devuelve la hora en prosa cuando no la sabe. Eso NO es una fecha.
test('una fecha que no es ISO se descarta y el resto sobrevive', () => {
  const e = eventoDe('{"titulo":"Dentista","inicio":"el jueves a las 2"}');
  expect(e?.titulo).toBe('Dentista');
  expect(e?.inicio).toBeUndefined();
});

test('un fin anterior al inicio se descarta', () => {
  const e = eventoDe('{"titulo":"X","inicio":"2026-09-17T14:00:00Z","fin":"2026-09-17T09:00:00Z"}');
  expect(e?.inicio).toBe('2026-09-17T14:00:00.000Z');
  expect(e?.fin).toBeUndefined();
});
