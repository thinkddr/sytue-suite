/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import type { FotoDeGaleria } from '../client/galeriaCifrada.js';
import { mezclarLinea } from './lineaMezclada.js';

const cifrada = (id: string, tomada: string): FotoDeGaleria => ({
  id,
  nombre: `${id}.jpg`,
  tipo: 'image/jpeg',
  bytes: 1,
  suma: '',
  conEntrada: true,
  pendiente: false,
  entrada: {
    v: 1,
    tipo: 'sytue.photos.cifrada.foto',
    foto: id,
    suma: '',
    nombre: `${id}.jpg`,
    tomada,
    exif: true,
    ancho: 1,
    alto: 1,
    favorita: false,
    albumes: [],
    miniatura: '',
    actualizada: '',
  },
});
const servidor = [
  { id: 's1', takenAt: '2026-06-01T00:00:00.000Z' },
  { id: 's2', takenAt: '2025-06-01T00:00:00.000Z' },
];
const cifradas = [
  cifrada('c1', '2026-08-01T00:00:00.000Z'),
  cifrada('c2', '2025-12-01T00:00:00.000Z'),
  cifrada('c3', '2019-01-01T00:00:00.000Z'),
];

test('junta por fecha y espera a que el servidor llegue hasta las más antiguas', () => {
  expect(mezclarLinea(servidor, cifradas, { fin: false }).map((e) => e.id)).toEqual([
    'c1',
    's1',
    'c2',
    's2',
  ]);
  expect(mezclarLinea(servidor, cifradas, { fin: true }).map((e) => e.id)).toEqual([
    'c1',
    's1',
    'c2',
    's2',
    'c3',
  ]);
  // Sin nada del servidor todavía, nada; y con la línea vacía terminada, todas.
  expect(mezclarLinea([], cifradas, { fin: false })).toEqual([]);
  expect(mezclarLinea([], cifradas, { fin: true }).map((e) => e.id)).toEqual(['c1', 'c2', 'c3']);
});

test('con un año elegido o una búsqueda', () => {
  expect(mezclarLinea(servidor, cifradas, { fin: true, anyo: 2025 }).map((e) => e.id)).toEqual([
    's1',
    'c2',
    's2',
    'c3',
  ]);
  expect(
    mezclarLinea(servidor, [cifradas[2]!], { fin: false, busqueda: true }).map((e) => e.id),
  ).toEqual(['c3', 's1', 's2']);
});
