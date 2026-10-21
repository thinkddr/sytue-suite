// Checks de las tarjetas de llamada (iniciada vs finalizada + duración). `bun test`.
import { expect, test } from 'bun:test';
import { callEndedSeconds, formatCallDuration } from './call.js';
import { roomsT } from '../i18n/index.js';

test('callEndedSeconds distingue finalizada (entero) de iniciada (vacío)', () => {
  expect(callEndedSeconds('')).toBeNull(); // «inició una llamada»
  expect(callEndedSeconds('   ')).toBeNull();
  expect(callEndedSeconds('0')).toBe(0); // finalizada al instante
  expect(callEndedSeconds('125')).toBe(125);
  expect(callEndedSeconds('abc')).toBeNull();
  expect(callEndedSeconds('12.5')).toBeNull();
});

test('formatCallDuration da etiquetas cortas en el idioma base (inglés)', () => {
  const t = roomsT('en');
  expect(formatCallDuration(t, 0)).toBe('less than 1 min');
  expect(formatCallDuration(t, 59)).toBe('less than 1 min');
  expect(formatCallDuration(t, 60)).toBe('1 min');
  expect(formatCallDuration(t, 125)).toBe('2 min');
  expect(formatCallDuration(t, 3600)).toBe('1 h');
  expect(formatCallDuration(t, 3900)).toBe('1 h 5 min');
});

test('formatCallDuration habla los cinco idiomas', () => {
  expect(formatCallDuration(roomsT('es'), 30)).toBe('menos de 1 min');
  expect(formatCallDuration(roomsT('fr'), 30)).toBe('moins d’une minute');
  expect(formatCallDuration(roomsT('de'), 30)).toBe('weniger als 1 Min.');
  expect(formatCallDuration(roomsT('it'), 30)).toBe('meno di 1 min');
});
