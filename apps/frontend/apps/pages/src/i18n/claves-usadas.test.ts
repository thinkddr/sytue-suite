/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NINGUNA CLAVE DEL CATÁLOGO SIN USAR, como www y la consola (@sytue/i18n/claves-usadas):
 * Sytue Web se traduce a 26 idiomas, y una clave que nadie pide se traduciría 26 veces
 * para nada. Las dinámicas van en `DINAMICAS` con las claves EXACTAS que producen.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';
import { clavesSinUsar, codigoDe, plantillasDinamicas } from '@sytue/i18n/claves-usadas';
import { SECCIONES } from '../components/Nav.js';
import { CATEGORIAS } from '../lib/galeria.js';
import { ALERGENOS_UE } from '../tienda/tipos.js';
import { en } from './messages/en.js';

const codigo = codigoDe(join(import.meta.dir, '..'));

const DINAMICAS: Record<string, readonly string[]> = {
  'nav.${s.clave}': SECCIONES.map((s) => `nav.${s.clave}`),
  // `otras` es la de las plantillas sin categoría (`p.category || 'otras'`).
  'new.cat.${c}': [...CATEGORIAS, 'otras'].map((c) => `new.cat.${c}`),
  'store.allergen.${id}': ALERGENOS_UE.map((id) => `store.allergen.${id}`),
};

test('toda plantilla dinámica del código está en DINAMICAS, y ninguna de más', () => {
  expect(plantillasDinamicas(codigo)).toEqual(
    Object.keys(DINAMICAS).sort((a, b) => a.localeCompare(b)),
  );
});

test('toda clave que produce una plantilla dinámica existe en el catálogo', () => {
  expect(
    Object.values(DINAMICAS)
      .flat()
      .filter((k) => !(k in en)),
  ).toEqual([]);
});

test('ninguna clave del catálogo se queda sin usar', () => {
  expect(clavesSinUsar(Object.keys(en), codigo, DINAMICAS)).toEqual([]);
});
