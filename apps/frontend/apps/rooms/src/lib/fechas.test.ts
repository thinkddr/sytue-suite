// Checks de las fechas del historial y los paneles (ROOM-09). Ejecutar: `bun test`.
// El reloj va inyectado: «ahora» es el lunes 9 de noviembre de 2026 a mediodía local.
import { expect, test } from 'bun:test';
import { roomsT } from '../i18n/index.js';
import { dayLabel, whenLabel } from './fechas.js';

const ahora = new Date(2026, 10, 9, 12, 0);

test('dayLabel: hoy y ayer siguen saliendo del catálogo', () => {
  for (const locale of ['es', 'de', 'en'] as const) {
    const t = roomsT(locale);
    expect(dayLabel(locale, t, new Date(2026, 10, 9, 8, 30), ahora)).toBe(t('day.today'));
    expect(dayLabel(locale, t, new Date(2026, 10, 8, 23, 59), ahora)).toBe(t('day.yesterday'));
  }
});

test('dayLabel: una fecha de este año no lleva año', () => {
  const d = new Date(2026, 2, 6, 10, 0);
  expect(dayLabel('es', roomsT('es'), d, ahora)).toBe('viernes, 6 de marzo');
  expect(dayLabel('de', roomsT('de'), d, ahora)).toBe('Freitag, 6. März');
  expect(dayLabel('en', roomsT('en'), d, ahora)).not.toContain('2026');
});

test('dayLabel: una fecha de otro año sí lleva el año', () => {
  const d = new Date(2025, 10, 7, 10, 0);
  expect(dayLabel('es', roomsT('es'), d, ahora)).toBe('viernes, 7 de noviembre de 2025');
  expect(dayLabel('de', roomsT('de'), d, ahora)).toBe('Freitag, 7. November 2025');
  const en = dayLabel('en', roomsT('en'), d, ahora);
  expect(en).toContain('Friday');
  expect(en).toContain('2025');
});

test('dayLabel: el año que manda es el del reloj, no el del sistema', () => {
  // Un 31 de diciembre visto desde el 1 de enero es «Ayer» aunque cambie el año.
  const nochevieja = new Date(2026, 11, 31, 22, 0);
  const t = roomsT('es');
  expect(dayLabel('es', t, nochevieja, new Date(2027, 0, 1, 9, 0))).toBe(t('day.yesterday'));
  expect(dayLabel('es', t, nochevieja, new Date(2027, 0, 5, 9, 0))).toContain('2026');
});

test('whenLabel: las filas de los paneles siguen la misma regla del año', () => {
  for (const locale of ['es', 'de', 'en'] as const) {
    const esteAno = whenLabel(locale, new Date(2026, 8, 24, 9, 5), ahora);
    expect(esteAno).not.toContain('2026');
    expect(esteAno).toContain('09:05');
    const otroAno = whenLabel(locale, new Date(2024, 8, 24, 9, 5), ahora);
    expect(otroAno).toContain('2024');
    expect(otroAno).toContain('09:05');
  }
});
