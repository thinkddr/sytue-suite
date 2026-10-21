import { expect, test } from 'bun:test';
import { haTerminado, limitesDelDia, ordenarEventos } from './hoy.js';

test('los límites del día van de medianoche local a la siguiente', () => {
  const { from, to } = limitesDelDia(new Date(2026, 8, 9, 15, 30));
  const a = new Date(from);
  const b = new Date(to);
  expect([a.getHours(), a.getMinutes(), a.getDate()]).toEqual([0, 0, 9]);
  expect(b.getTime() - a.getTime()).toBe(24 * 3600 * 1000);
});

test('todo el día primero y luego por hora, sin tocar la lista original', () => {
  const lista = [
    { id: 'b', summary: 'B', start: '2026-09-09T11:00:00Z', allDay: false },
    { id: 'c', summary: 'C', start: '2026-09-09', allDay: true },
    { id: 'a', summary: 'A', start: '2026-09-09T09:00:00Z', allDay: false },
  ];
  expect(ordenarEventos(lista).map((e) => e.id)).toEqual(['c', 'a', 'b']);
  expect(lista.map((e) => e.id)).toEqual(['b', 'c', 'a']);
});

test('un evento con fin pasado ha terminado; uno de todo el día, nunca', () => {
  const ahora = new Date('2026-09-09T12:00:00Z');
  expect(
    haTerminado(
      {
        id: 'x',
        summary: 'x',
        start: '2026-09-09T09:00:00Z',
        end: '2026-09-09T10:00:00Z',
        allDay: false,
      },
      ahora,
    ),
  ).toBe(true);
  expect(haTerminado({ id: 'y', summary: 'y', start: '2026-09-09', allDay: true }, ahora)).toBe(
    false,
  );
  expect(
    haTerminado({ id: 'z', summary: 'z', start: '2026-09-09T13:00:00Z', allDay: false }, ahora),
  ).toBe(false);
});
