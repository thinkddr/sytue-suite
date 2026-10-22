/**
 * Matemática del desgaje de series («este y los siguientes»): la serie nueva
 * hereda el patrón con el fin recalculado (COUNT restante / UNTIL) y arrastra los
 * EXDATE posteriores al corte. El recorte del maestro (EXDATE/UNTIL) se prueba en
 * Go (services/calendar/internal/caldav). Aquí, las funciones puras del cliente.
 */
import { describe, expect, test } from 'bun:test';
import { carriedExdates, carryRule, countBefore } from './recurrence.js';
import type { Recurrence } from '../client/calendarApi.js';

const masterStart = new Date('2026-07-01T09:00:00Z'); // 1-jul (miércoles)
const cut = new Date('2026-07-15T09:00:00Z'); // corte en la 3ª ocurrencia semanal

describe('countBefore', () => {
  test('cuenta las ocurrencias semanales anteriores al corte', () => {
    expect(countBefore(masterStart, { freq: 'WEEKLY' }, cut)).toBe(2); // 1-jul, 8-jul
  });
  test('no cuenta más allá del COUNT del maestro', () => {
    expect(countBefore(masterStart, { freq: 'WEEKLY', count: 2 }, cut)).toBe(2);
  });
});

describe('carryRule', () => {
  test('COUNT: la serie nueva hereda el remanente (10 − 2 = 8)', () => {
    const rec: Recurrence = { freq: 'WEEKLY', count: 10 };
    expect(carryRule(rec, masterStart, cut)).toEqual({ freq: 'WEEKLY', count: 8 });
  });
  test('UNTIL: la serie nueva conserva el UNTIL del maestro', () => {
    const rec: Recurrence = { freq: 'WEEKLY', until: '2026-12-31' };
    expect(carryRule(rec, masterStart, cut)).toEqual({ freq: 'WEEKLY', until: '2026-12-31' });
  });
  test('infinita: solo freq (+ interval si >1)', () => {
    expect(carryRule({ freq: 'DAILY', interval: 2 }, masterStart, cut)).toEqual({
      freq: 'DAILY',
      interval: 2,
    });
  });
  test('remanente agotado se acota a 1 (nunca 0 ni negativo)', () => {
    const rec: Recurrence = { freq: 'WEEKLY', count: 2 };
    expect(carryRule(rec, masterStart, new Date('2026-08-05T09:00:00Z')).count).toBe(1);
  });
});

describe('carriedExdates', () => {
  test('arrastra solo los EXDATE en/después del corte', () => {
    const ical =
      'BEGIN:VEVENT\r\n' +
      'EXDATE:20260708T090000Z\r\n' + // antes del corte → se queda en el maestro
      'EXDATE:20260722T090000Z\r\n' + // después → se arrastra a la serie nueva
      'END:VEVENT';
    expect(carriedExdates(ical, cut)).toEqual(['20260722T090000Z']);
  });
});
