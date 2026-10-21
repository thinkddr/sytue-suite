// Checks del cálculo de los atajos de «Enviar más tarde». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { schedulePresets, toLocalValue } from './schedule.js';

/** Lee un valor `datetime-local` como Date en hora LOCAL (lo que hace el navegador). */
const parse = (v: string): Date => new Date(v);

test('toLocalValue: devuelve la hora LOCAL sin zona, en formato del control', () => {
  const d = new Date('2026-07-15T10:30:00'); // interpretado como local
  expect(toLocalValue(d.getTime())).toBe('2026-07-15T10:30');
});

test('schedulePresets: «mañana» cae al día siguiente a las 9:00 locales', () => {
  const now = new Date('2026-07-15T22:40:00').getTime(); // miércoles, ya de noche
  const [tomorrow] = schedulePresets(now);
  expect(tomorrow!.key).toBe('schedule.tomorrow');
  expect(tomorrow!.value).toBe('2026-07-16T09:00');
  expect(parse(tomorrow!.value).getTime()).toBeGreaterThan(now);
});

test('schedulePresets: «el lunes» es el próximo lunes a las 9:00', () => {
  const now = new Date('2026-07-15T10:00:00').getTime(); // miércoles
  const [, monday] = schedulePresets(now);
  expect(monday!.key).toBe('schedule.monday');
  const d = parse(monday!.value);
  expect(d.getDay()).toBe(1);
  expect(d.getHours()).toBe(9);
  expect(d.getTime()).toBeGreaterThan(now);
});

test('schedulePresets: en lunes, «el lunes» salta al de la semana siguiente', () => {
  const now = new Date('2026-07-13T08:00:00').getTime(); // lunes, antes de las 9
  const [, monday] = schedulePresets(now);
  const d = parse(monday!.value);
  expect(d.getDay()).toBe(1);
  expect(d.getDate()).toBe(20); // no «hoy a las 9», el lunes siguiente
});

test('schedulePresets: ambos atajos son siempre futuros, cualquier día de la semana', () => {
  for (let day = 12; day <= 18; day++) {
    const now = new Date(`2026-07-${day}T23:59:00`).getTime();
    for (const p of schedulePresets(now)) {
      expect(parse(p.value).getTime()).toBeGreaterThan(now);
    }
  }
});
