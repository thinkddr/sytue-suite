// Self-check de groupByDay: corta la actividad por día LOCAL, conserva el orden
// del Hub y no inventa fecha para lo que no la trae.
// Ejecutar: cd apps/frontend/apps/portal && bun test
// (Fuera de src/ a propósito: astro check no typechequea este fichero.)
import { expect, test } from 'bun:test';
import { dayLabelKey, groupByDay } from '../src/lib/activity.js';
import type { SearchResult } from '@sytue/api-clients';

const hit = (id: string, timestamp?: number): SearchResult => ({
  source: 'mail',
  id,
  title: id,
  url: `/${id}`,
  ...(timestamp !== undefined ? { timestamp } : {}),
});

/** Instante de hoy a mediodía, para que el día local no dependa de la hora del run. */
const hoyA = (h: number): number => {
  const d = new Date();
  d.setHours(h, 0, 0, 0);
  return d.getTime();
};
const diasAtras = (n: number, h = 12): number => {
  const d = new Date();
  d.setDate(d.getDate() - n);
  d.setHours(h, 0, 0, 0);
  return d.getTime();
};

test('agrupa por día y conserva el orden que trae el Hub', () => {
  const grupos = groupByDay([
    hit('a', hoyA(10)),
    hit('b', hoyA(9)),
    hit('c', diasAtras(1)),
    hit('d', diasAtras(3)),
  ]);
  expect(grupos.length).toBe(3);
  expect(grupos[0]!.items.map((i) => i.id)).toEqual(['a', 'b']);
  expect(grupos[1]!.items.map((i) => i.id)).toEqual(['c']);
  expect(grupos[2]!.items.map((i) => i.id)).toEqual(['d']);
});

test('lo que no trae fecha va a un grupo propio, y al final', () => {
  const grupos = groupByDay([hit('sin-fecha'), hit('hoy', hoyA(11))]);
  expect(grupos[grupos.length - 1]!.day).toBe('');
  expect(grupos[grupos.length - 1]!.items.map((i) => i.id)).toEqual(['sin-fecha']);
  // Y no se mezcla con el día real: son dos grupos, no uno.
  expect(grupos.length).toBe(2);
});

test('el día se calcula en hora LOCAL, no en UTC', () => {
  // A las 00:30 de una zona por delante de UTC, el instante sigue siendo HOY:
  // con toISOString() el encabezado diría «ayer».
  const grupos = groupByDay([hit('madrugada', hoyA(0) + 30 * 60_000)]);
  expect(dayLabelKey(grupos[0]!.day)).toBe('today');
});

test('etiqueta hoy, ayer y fecha', () => {
  const hoy = new Date();
  const ayer = new Date();
  ayer.setDate(ayer.getDate() - 1);
  const clave = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

  expect(dayLabelKey(clave(hoy), hoy)).toBe('today');
  expect(dayLabelKey(clave(ayer), hoy)).toBe('yesterday');
  expect(dayLabelKey('2020-01-01', hoy)).toBe('date');
  // Sin fecha no es «hoy» por descarte.
  expect(dayLabelKey('', hoy)).toBe('date');
});
