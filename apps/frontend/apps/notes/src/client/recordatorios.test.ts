// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { expect, test } from 'bun:test';
import {
  REPETICIONES,
  entradaRecordatorio,
  instanteDe,
  repeticionDe,
  valorLocal,
  yaPaso,
} from './recordatorios.js';

test('cada repetición del selector vuelve a leerse como la misma', () => {
  for (const rep of REPETICIONES) {
    const e = entradaRecordatorio('2026-10-03T09:30', false, rep, 3, '', 'Europe/Madrid');
    expect(e).not.toBeNull();
    expect(repeticionDe(e!)).toBe(rep);
  }
  expect(entradaRecordatorio('2026-10-03', true, 'weekdays', 0, '', 'UTC')!.rrule).toBe(
    'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR',
  );
});

test('una RRULE ajena se respeta al guardar, no se pierde', () => {
  const ajena = 'FREQ=YEARLY;BYMONTH=3';
  expect(repeticionDe({ rrule: ajena })).toBe('custom');
  expect(entradaRecordatorio('2026-10-03T09:30', false, 'custom', 0, ajena)!.rrule).toBe(ajena);
});

test('la flexible exige de 1 a 365 días y no lleva RRULE', () => {
  expect(entradaRecordatorio('2026-10-03T09:30', false, 'flexible', 0)).toBeNull();
  expect(entradaRecordatorio('2026-10-03T09:30', false, 'flexible', 366)).toBeNull();
  const e = entradaRecordatorio('2026-10-03T09:30', false, 'flexible', 2)!;
  expect([e.rrule, e.after_days]).toEqual(['', 2]);
});

test('todo el día avisa a las 9 de su día, y el valor del control da la vuelta', () => {
  const d = instanteDe('2026-10-03', true)!;
  expect([d.getFullYear(), d.getMonth(), d.getDate(), d.getHours(), d.getMinutes()]).toEqual([
    2026, 9, 3, 9, 0,
  ]);
  expect(valorLocal(d.toISOString(), true)).toBe('2026-10-03');
  const h = instanteDe('2026-10-03T21:05', false)!;
  expect(valorLocal(h.toISOString(), false)).toBe('2026-10-03T21:05');
  expect(instanteDe('mañana', false)).toBeNull();
});

test('un recordatorio pasado se pinta como pasado, uno futuro no', () => {
  const ahora = Date.parse('2026-10-03T10:00:00Z');
  expect(yaPaso({ due_at: '2026-10-03T09:59:00Z' }, ahora)).toBe(true);
  expect(yaPaso({ due_at: '2026-10-03T10:01:00Z' }, ahora)).toBe(false);
});
