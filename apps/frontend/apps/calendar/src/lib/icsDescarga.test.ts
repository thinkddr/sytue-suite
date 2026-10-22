import { expect, test } from 'bun:test';
import { icsCompleto, nombreIcs } from './icsDescarga';

test('el nombre se sanea, se acorta y cae a «evento»', () => {
  expect(nombreIcs('Comité: 12/09')).toBe('Comité_ 12_09.ics');
  expect(nombreIcs('   ')).toBe('evento.ics');
  expect(nombreIcs('x'.repeat(200))).toBe(`${'x'.repeat(120)}.ics`);
});

test('un VCALENDAR se deja intacto', () => {
  const ics =
    'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:1\r\nEND:VEVENT\r\nEND:VCALENDAR';
  expect(icsCompleto(ics)).toBe(ics);
});

test('un VEVENT suelto se envuelve para que un calendario pueda abrirlo', () => {
  const salida = icsCompleto('BEGIN:VEVENT\r\nUID:1\r\nEND:VEVENT');
  expect(salida.startsWith('BEGIN:VCALENDAR')).toBe(true);
  expect(salida.endsWith('END:VCALENDAR')).toBe(true);
  expect(salida).toContain('BEGIN:VEVENT');
  expect(salida).toContain('VERSION:2.0');
});
