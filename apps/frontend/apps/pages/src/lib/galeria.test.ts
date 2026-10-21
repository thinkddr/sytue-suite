/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { describe, expect, it } from 'bun:test';
import { agrupar, buscar, CATEGORIAS, puntuar, type PlantillaInfo } from './galeria';

const p = (
  key: string,
  es: string,
  en: string,
  category: string,
  kw: [string, string],
): PlantillaInfo => ({
  key,
  name: { es, en },
  description: { es: '', en: '' },
  vertical: key,
  category,
  keywords: { es: kw[0], en: kw[1] },
  store: false,
});
const LISTA = [
  p('peluqueria', 'Peluquería', 'Hair salon', 'belleza', [
    'pelu peluquero cortes tinte',
    'hairdresser haircut',
  ]),
  p('barberia', 'Barbería', 'Barber shop', 'belleza', [
    'barbero barba afeitado navaja',
    'barber beard',
  ]),
  p('fontanero', 'Fontanero', 'Plumber', 'oficios', [
    'fontaneria tuberias fugas urgencias',
    'plumbing leaks',
  ]),
  p('bar-cafeteria', 'Bar y cafetería', 'Bar and café', 'comida', [
    'bar cafe desayunos tapas',
    'coffee breakfast',
  ]),
  p('antigua', 'Antigua', 'Old', '', ['', '']),
];
const etiqueta = (c: string) => c;

describe('buscador de la galería', () => {
  it('encuentra por diminutivos y sinónimos de uso común', () => {
    expect(buscar(LISTA, 'pelu', null, 'es', etiqueta).map((x) => x.key)).toEqual(['peluqueria']);
    expect(buscar(LISTA, 'barbero', null, 'es', etiqueta).map((x) => x.key)).toEqual(['barberia']);
    expect(buscar(LISTA, 'tuberias', null, 'es', etiqueta).map((x) => x.key)).toEqual([
      'fontanero',
    ]);
  });

  it('no distingue tildes ni mayúsculas y admite plurales', () => {
    expect(buscar(LISTA, 'PELUQUERÍAS', null, 'es', etiqueta).map((x) => x.key)).toEqual([
      'peluqueria',
    ]);
    expect(buscar(LISTA, 'cafeteria', null, 'es', etiqueta).map((x) => x.key)).toEqual([
      'bar-cafeteria',
    ]);
    expect(buscar(LISTA, 'cafe', null, 'es', etiqueta).map((x) => x.key)).toEqual([
      'bar-cafeteria',
    ]);
  });

  it('busca también en inglés', () => {
    expect(buscar(LISTA, 'plumber', null, 'en', etiqueta).map((x) => x.key)).toEqual(['fontanero']);
    expect(buscar(LISTA, 'haircut', null, 'en', etiqueta).map((x) => x.key)).toEqual([
      'peluqueria',
    ]);
  });

  it('todas las palabras tienen que aparecer y el nombre pesa más que las claves', () => {
    expect(buscar(LISTA, 'bar cafe', null, 'es', etiqueta).map((x) => x.key)).toEqual([
      'bar-cafeteria',
    ]);
    expect(buscar(LISTA, 'bar zzz', null, 'es', etiqueta)).toEqual([]);
    // «bar» es palabra entera en «Bar y cafetería» y solo el inicio de «Barbería»: gana la entera
    expect(buscar(LISTA, 'bar', null, 'es', etiqueta).map((x) => x.key)).toEqual([
      'bar-cafeteria',
      'barberia',
    ]);
    // «barba» solo está en las claves de la barbería
    expect(buscar(LISTA, 'barba', null, 'es', etiqueta).map((x) => x.key)).toEqual(['barberia']);
  });

  it('filtra por categoría y sin consulta devuelve todas ordenadas por nombre', () => {
    expect(buscar(LISTA, '', 'belleza', 'es', etiqueta).map((x) => x.key)).toEqual([
      'barberia',
      'peluqueria',
    ]);
    expect(buscar(LISTA, '  ', null, 'es', etiqueta)).toHaveLength(5);
  });

  it('puntúa 0 lo que no coincide', () => {
    expect(puntuar(LISTA[0]!, 'zapatos', 'belleza')).toBe(0);
  });
});

describe('agrupación', () => {
  it('sigue el orden del catálogo y deja las categorías sin clasificar al final', () => {
    const g = agrupar(LISTA, 'es');
    expect(g.map((x) => x.categoria)).toEqual(['belleza', 'comida', 'oficios', 'otras']);
    expect(g[0]?.plantillas.map((x) => x.key)).toEqual(['barberia', 'peluqueria']);
  });

  it('hay catorce categorías y ninguna repetida', () => {
    expect(new Set(CATEGORIAS).size).toBe(CATEGORIAS.length);
    expect(CATEGORIAS).toHaveLength(14);
  });
});
