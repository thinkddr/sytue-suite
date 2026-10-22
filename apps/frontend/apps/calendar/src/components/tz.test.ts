/**
 * Zonas horarias y resolución de instantes a la ZONA DE VISUALIZACIÓN. Tests
 * DETERMINISTAS: usan instantes absolutos (getTime) e `Intl` con timeZone
 * explícito, así que no dependen de la zona del runner. Cubren el DST real
 * (Europe/Madrid, America/New_York) y que los cálculos de layout reciben el
 * instante correcto (hora-de-pared de la zona de visualización).
 */
import { describe, expect, test } from 'bun:test';
import {
  browserZone,
  listTimeZones,
  zoneOffsetMinutes,
  zoneLabel,
  instantFromZonedWallClock,
} from './tz.js';
import { parseTzid, eventInstant } from './dates.js';
import { dayMinutes } from './timeLayout.js';

/** Hora-de-pared (0..23) de un instante en una zona (helper de test, Intl nativo). */
function wallHour(at: Date, tz: string): number {
  return Number(
    new Intl.DateTimeFormat('en-US', { timeZone: tz, hour: '2-digit', hourCycle: 'h23' }).format(
      at,
    ),
  );
}

describe('zoneOffsetMinutes', () => {
  test('Europe/Madrid: +60 en invierno (CET), +120 en verano (CEST)', () => {
    expect(zoneOffsetMinutes(new Date(Date.UTC(2025, 0, 15)), 'Europe/Madrid')).toBe(60);
    expect(zoneOffsetMinutes(new Date(Date.UTC(2025, 6, 15)), 'Europe/Madrid')).toBe(120);
  });
  test('America/New_York: -300 (EST) / -240 (EDT)', () => {
    expect(zoneOffsetMinutes(new Date(Date.UTC(2025, 0, 15)), 'America/New_York')).toBe(-300);
    expect(zoneOffsetMinutes(new Date(Date.UTC(2025, 6, 15)), 'America/New_York')).toBe(-240);
  });
});

describe('zoneLabel', () => {
  test('nombre legible + offset actual', () => {
    expect(zoneLabel('Europe/Madrid', new Date(Date.UTC(2025, 6, 1)))).toBe(
      'Europe/Madrid (GMT+02:00)',
    );
    expect(zoneLabel('America/New_York', new Date(Date.UTC(2025, 0, 1)))).toBe(
      'America/New York (GMT-05:00)',
    );
  });
});

describe('instantFromZonedWallClock (DST)', () => {
  test('09:00 Madrid: invierno = 08:00Z (CET), verano = 07:00Z (CEST)', () => {
    // Misma hora local, distinto instante UTC a ambos lados del cambio de hora.
    expect(instantFromZonedWallClock(2025, 1, 15, 9, 0, 0, 'Europe/Madrid').getTime()).toBe(
      Date.UTC(2025, 0, 15, 8, 0, 0),
    );
    expect(instantFromZonedWallClock(2025, 3, 31, 9, 0, 0, 'Europe/Madrid').getTime()).toBe(
      Date.UTC(2025, 2, 31, 7, 0, 0),
    );
  });
  test('09:00 New York: invierno = 14:00Z (EST), verano = 13:00Z (EDT)', () => {
    expect(instantFromZonedWallClock(2025, 3, 3, 9, 0, 0, 'America/New_York').getTime()).toBe(
      Date.UTC(2025, 2, 3, 14, 0, 0),
    );
    expect(instantFromZonedWallClock(2025, 3, 10, 9, 0, 0, 'America/New_York').getTime()).toBe(
      Date.UTC(2025, 2, 10, 13, 0, 0),
    );
  });
});

describe('parseTzid', () => {
  test('extrae el TZID del DTSTART', () => {
    expect(parseTzid('BEGIN:VEVENT\r\nDTSTART;TZID=Europe/Madrid:20250331T090000\r\n')).toBe(
      'Europe/Madrid',
    );
  });
  test('null si el DTSTART es UTC o de día completo', () => {
    expect(parseTzid('DTSTART:20250331T070000Z\r\n')).toBeNull();
    expect(parseTzid('DTSTART;VALUE=DATE:20250331\r\n')).toBeNull();
  });
});

describe('eventInstant', () => {
  test('TZID: reloj-de-pared en la zona del evento -> instante absoluto', () => {
    expect(eventInstant('20250331T090000', 'Europe/Madrid')!.getTime()).toBe(
      Date.UTC(2025, 2, 31, 7, 0, 0),
    );
  });
  test('UTC (...Z): instante directo, ignora el TZID', () => {
    expect(eventInstant('20250331T070000Z', 'Europe/Madrid')!.getTime()).toBe(
      Date.UTC(2025, 2, 31, 7, 0, 0),
    );
  });
  test('día completo (yyyymmdd): flotante, medianoche local (mismo día)', () => {
    const d = eventInstant('20250331', null)!;
    expect(d.getFullYear()).toBe(2025);
    expect(d.getMonth()).toBe(2);
    expect(d.getDate()).toBe(31);
    expect(d.getHours()).toBe(0);
  });
});

describe('layout con zona de visualización', () => {
  // Un evento «09:00 Europe/Madrid» se sitúa por la hora-de-pared de la ZONA DE
  // VISUALIZACIÓN: 09:00 vista desde Madrid; otra hora vista desde Nueva York.
  const inst = eventInstant('20250331T090000', 'Europe/Madrid')!;
  test('posiciona a las 09:00 en la zona del propio evento', () => {
    expect(wallHour(inst, 'Europe/Madrid')).toBe(9);
    // 07:00Z en Nueva York (EDT, -4) = 03:00.
    expect(wallHour(inst, 'America/New_York')).toBe(3);
  });
  test('dayMinutes recibe el instante ya resuelto (posición estable)', () => {
    // Reafirma que el instante resuelto entra en el layout puro sin recalcular
    // zona: para el mismo día natural del inicio, cae dentro de [0, 1440).
    const day = new Date(inst);
    day.setHours(12, 0, 0, 0);
    const iv = dayMinutes(inst, new Date(inst.getTime() + 3_600_000), day);
    expect(iv).not.toBeNull();
    expect(iv!.endMin - iv!.startMin).toBe(60);
  });
  test('browserZone/listTimeZones disponibles', () => {
    expect(typeof browserZone()).toBe('string');
    expect(listTimeZones().length).toBeGreaterThan(0);
  });
});
