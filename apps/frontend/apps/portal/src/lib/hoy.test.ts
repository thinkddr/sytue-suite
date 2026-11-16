import { expect, test } from 'bun:test';
import {
  estadoDelEvento,
  eventosDeMas,
  haTerminado,
  limitesDelDia,
  marcasDeLaAgenda,
  MAX_EVENTOS,
  ordenarEventos,
  recortarAgenda,
  rutaDelEvento,
  type EventoDeHoy,
} from './hoy.js';

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

test('cada evento abre su día en Calendar, con la fecha de la instancia', () => {
  expect(
    rutaDelEvento({ id: 'x', summary: 'x', start: '2026-09-09T09:00:00Z', allDay: false }),
  ).toBe('/?date=2026-09-09T09%3A00%3A00Z');
  // Todo el día: `all_day=1`, y la fecha compacta de iCalendar, con guiones.
  expect(rutaDelEvento({ id: 'y', summary: 'y', start: '20260909', allDay: true })).toBe(
    '/?date=2026-09-09&all_day=1',
  );
});

// Reloj fijo: las 12:00 del 9 de septiembre de 2026.
const MEDIODIA = new Date('2026-09-09T12:00:00Z');
const aLas = (id: string, desde: string, hasta?: string): EventoDeHoy => ({
  id,
  summary: id,
  start: `2026-09-09T${desde}:00Z`,
  ...(hasta ? { end: `2026-09-09T${hasta}:00Z` } : {}),
  allDay: false,
});

test('el estado de un evento: pasado, en curso o por venir; todo el día, ninguno', () => {
  expect(estadoDelEvento(aLas('a', '09:00', '10:00'), MEDIODIA)).toBe('pasado');
  expect(estadoDelEvento(aLas('b', '11:30', '12:30'), MEDIODIA)).toBe('ahora');
  // El instante exacto del inicio ya cuenta como en curso.
  expect(estadoDelEvento(aLas('c', '12:00', '13:00'), MEDIODIA)).toBe('ahora');
  expect(estadoDelEvento(aLas('d', '15:00', '16:00'), MEDIODIA)).toBe('siguiente');
  expect(
    estadoDelEvento({ id: 'e', summary: 'e', start: '2026-09-09', allDay: true }, MEDIODIA),
  ).toBe(null);
  // Sin fin no se sabe si sigue: ni «Ahora» ni pasado.
  expect(estadoDelEvento(aLas('f', '11:00'), MEDIODIA)).toBe(null);
});

test('«Ahora» en lo que está en curso y «Siguiente» solo en el primero por venir', () => {
  const lista = [
    aLas('pasado', '09:00', '10:00'),
    aLas('curso', '11:30', '12:30'),
    aLas('luego', '15:00', '16:00'),
    aLas('mas-tarde', '17:00', '18:00'),
  ];
  expect(marcasDeLaAgenda(lista, MEDIODIA)).toEqual([null, 'ahora', 'siguiente', null]);
});

test('con nueve eventos se ven seis y sobran tres; un calendario caído marca la agenda', () => {
  const nueve = Array.from({ length: 9 }, (_, i) =>
    aLas(`e${i}`, `${String(20 - i).padStart(2, '0')}:00`),
  );
  const r = recortarAgenda(nueve, 0, 2);
  expect(r?.eventos.length).toBe(MAX_EVENTOS);
  // Se recorta DESPUÉS de ordenar: los seis primeros del día, no los seis que llegaron.
  expect(r?.eventos[0]?.id).toBe('e8');
  expect(r?.total).toBe(9);
  expect(r?.parcial).toBe(false);
  expect(eventosDeMas(r?.total, r?.eventos.length ?? 0)).toBe(3);

  expect(recortarAgenda(nueve.slice(0, 2), 1, 3)?.parcial).toBe(true);
  // Sin un solo calendario que conteste no hay agenda: «no disponible», no un día vacío.
  expect(recortarAgenda([], 2, 2)).toBe(null);
  // Sin calendarios, sí: un día vacío de verdad.
  expect(recortarAgenda([], 0, 0)).toEqual({ eventos: [], total: 0, parcial: false });
  // Una respuesta vieja sin `total` no inventa un «+N».
  expect(eventosDeMas(undefined, 4)).toBe(0);
});
