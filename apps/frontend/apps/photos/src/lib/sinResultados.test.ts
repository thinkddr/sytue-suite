/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { sinResultados } from './sinResultados.js';

const base = {
  texto: ' playa ',
  cargando: false,
  pendiente: false,
  fotos: 0,
  error: false,
  tambien: 0,
};

test('una búsqueda sin aciertos dice qué se buscó y ofrece ampliar', () => {
  expect(sinResultados(base)).toEqual({ consulta: 'playa', ampliar: true });
});

test('ya ampliada, sigue el vacío pero no se ofrece ampliar otra vez', () => {
  expect(sinResultados({ ...base, tambien: 2 })).toEqual({ consulta: 'playa', ampliar: false });
});

test('sin búsqueda, cargando, con fotos o con un fallo no hay «sin resultados»', () => {
  expect(sinResultados({ ...base, texto: '  ' })).toBeNull();
  expect(sinResultados({ ...base, cargando: true })).toBeNull();
  expect(sinResultados({ ...base, fotos: 3 })).toBeNull();
  expect(sinResultados({ ...base, error: true })).toBeNull();
});

test('mientras la búsqueda espera al temporizador no dice que no hay nada', () => {
  expect(sinResultados({ ...base, pendiente: true })).toBeNull();
});
