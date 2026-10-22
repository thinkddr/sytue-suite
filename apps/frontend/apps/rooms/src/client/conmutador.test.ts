import { expect, test } from 'bun:test';
import { ordenarCanales } from './conmutador.js';

const canales = [
  { id: '1', name: 'general', unread: 0 },
  { id: '2', name: 'Facturación', unread: 3 },
  { id: '3', name: 'diseño', unread: 0, mentions: 1 },
  { id: '4', name: 'aleatorio', unread: 0 },
];

test('sin texto: lo pendiente primero y luego por nombre', () => {
  expect(ordenarCanales(canales, '').map((c) => c.id)).toEqual(['3', '2', '4', '1']);
});

test('con texto: empieza-por antes que contiene, sin acentos ni mayúsculas', () => {
  expect(ordenarCanales(canales, 'fact').map((c) => c.name)).toEqual(['Facturación']);
  expect(ordenarCanales(canales, 'DISENO').map((c) => c.name)).toEqual(['diseño']);
  expect(ordenarCanales(canales, 'a').map((c) => c.name)).toEqual([
    'aleatorio',
    'Facturación',
    'general',
  ]);
  expect(ordenarCanales(canales, 'zzz')).toEqual([]);
});
