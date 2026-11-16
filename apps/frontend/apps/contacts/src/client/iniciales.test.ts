/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las cabeceras A–Z (CONT-05) contra los vectores COMPARTIDOS con Fyne
 * (apps/fyne/contactos/testdata/iniciales.json, que recorre `TestInicialesSonLasDeLaWeb`).
 * Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-12): con la primera letra sin tilde a secas, la Ñ caía en la N
 * y la Å sueca en la A; sin la prueba contra la A–Z, la Ø inglesa tenía cabecera propia;
 * sin mandar «#» al final, las cifras abrían la lista. Deshecho.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { isLocale } from '@sytue/i18n';
import { gruposPorInicial, listaConCabeceras } from './iniciales.js';

interface Caso {
  nombre: string;
  locale: string;
  nombres: string[];
  grupos: { inicial: string; nombres: string[] }[];
}

const vectores = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/contactos/testdata/iniciales.json', import.meta.url),
    'utf8',
  ),
) as { casos: Caso[] };

test('hay vectores que recorrer', () => {
  expect(vectores.casos.length).toBeGreaterThanOrEqual(10);
});

for (const c of vectores.casos) {
  test(c.nombre, () => {
    if (!isLocale(c.locale)) throw new Error(`idioma desconocido ${c.locale}`);
    const grupos = gruposPorInicial(c.nombres, (n) => n, c.locale);
    expect(grupos.map((g) => ({ inicial: g.inicial, nombres: g.items }))).toEqual(c.grupos);
  });
}

test('con cabeceras: cada contacto sabe su fila y las cifras pasan al final', () => {
  const l = listaConCabeceras(['1 Taxi', 'Ana', 'Alba', 'Bea'], (n) => n, 'es', true);
  expect(l.items).toEqual(['Ana', 'Alba', 'Bea', '1 Taxi']);
  expect(l.filas).toEqual([
    { tipo: 'cabecera', inicial: 'A' },
    { tipo: 'contacto', indice: 0 },
    { tipo: 'contacto', indice: 1 },
    { tipo: 'cabecera', inicial: 'B' },
    { tipo: 'contacto', indice: 2 },
    { tipo: 'cabecera', inicial: '#' },
    { tipo: 'contacto', indice: 3 },
  ]);
  expect(l.filaDe).toEqual([1, 2, 4, 6]);
});

test('sin cabeceras (búsqueda o filtro): una fila por contacto, en el mismo orden', () => {
  const l = listaConCabeceras(['1 Taxi', 'Ana', 'Bea'], (n) => n, 'es', false);
  expect(l.items).toEqual(['1 Taxi', 'Ana', 'Bea']);
  expect(l.filas.every((f) => f.tipo === 'contacto')).toBe(true);
  expect(l.filaDe).toEqual([0, 1, 2]);
});
