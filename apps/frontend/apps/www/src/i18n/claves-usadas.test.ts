/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NINGUNA CLAVE DEL CATÁLOGO SIN USAR (2026-09-26).
 *
 * Una clave que ninguna página pide se sigue traduciendo 26 veces y nadie la ve:
 * `platform.<id>.format` sobrevivió así a la tarjeta que la enseñaba, y con ella
 * otras siete. Este test compara el catálogo inglés —la fuente de verdad, los otros
 * 25 no pueden tener claves de más porque su tipo sale de él— con lo que el código
 * pide de verdad:
 *
 *   - Una clave LITERAL cuenta como usada si aparece entre comillas en el código
 *     del sitio (no en los catálogos ni en los tests).
 *   - Una clave DINÁMICA (`t(\`product.${key}.tagline\`)`) no aparece nunca escrita,
 *     así que va en `DINAMICAS` con la lista EXACTA de claves que produce. Si
 *     alguien añade una plantilla nueva sin listarla, el primer test lo dice.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';
import { clavesSinUsar, codigoDe, plantillasDinamicas } from '@sytue/i18n/claves-usadas';
import { DESKTOP, MOBILE } from '../lib/downloads';
import { FEATURE_COUNT, PRODUCT_KEYS } from '../lib/product-content';
import { en } from './messages/en';

const codigo = codigoDe(join(import.meta.dir, '..', '..'));

// Estas listas repiten las de las páginas legales (`const sections = [...]`): un
// `.astro` no se puede importar desde Bun. Si una página gana una sección, su clave
// sale aquí como «sin usar» hasta que se añade.
const A11Y_SECCIONES = ['standard', 'status', 'limits'];
const A11Y_MEDIDAS = ['design', 'ci', 'system', 'text'];
const AVISO = ['scope', 'ip', 'liability', 'law'];
const TERMS = ['provider', 'scope', 'accounts', 'use', 'ip', 'liability', 'changes', 'law'];
const PRIVACY = [
  'basis',
  'retention',
  'processors',
  'security',
  'cookies',
  'transfers',
  'minors',
  'changes',
];
const PLATAFORMAS = [...DESKTOP, ...MOBILE].map((p) => p.id);

/** Cada `t(\`…${…}…\`)` del código, tal cual está escrito, y las claves que produce. */
const DINAMICAS: Record<string, readonly string[]> = {
  'product.${key}.blurb': PRODUCT_KEYS.map((k) => `product.${k}.blurb`),
  'product.${key}.tagline': PRODUCT_KEYS.map((k) => `product.${k}.tagline`),
  'product.${key}.standard': PRODUCT_KEYS.map((k) => `product.${k}.standard`),
  'product.${key}.p1': PRODUCT_KEYS.map((k) => `product.${k}.p1`),
  'product.${key}.p2': PRODUCT_KEYS.map((k) => `product.${k}.p2`),
  'product.${key}.p3': PRODUCT_KEYS.map((k) => `product.${k}.p3`),
  'product.${key}.p4': PRODUCT_KEYS.map((k) => `product.${k}.p4`),
  'product.${key}.f${i + 1}': PRODUCT_KEYS.flatMap((k) =>
    Array.from({ length: FEATURE_COUNT[k] }, (_, i) => `product.${k}.f${i + 1}`),
  ),
  'platform.${target.id}.req': PLATAFORMAS.map((id) => `platform.${id}.req`),
  'a11y.${key}.title': A11Y_SECCIONES.map((k) => `a11y.${k}.title`),
  'a11y.${key}.body': A11Y_SECCIONES.map((k) => `a11y.${k}.body`),
  'a11y.measures.${key}': A11Y_MEDIDAS.map((k) => `a11y.measures.${k}`),
  'aviso.${key}.title': AVISO.map((k) => `aviso.${k}.title`),
  'aviso.${key}.body': AVISO.map((k) => `aviso.${k}.body`),
  'terms.${key}.title': TERMS.map((k) => `terms.${k}.title`),
  'terms.${key}.body': TERMS.map((k) => `terms.${k}.body`),
  'privacy.${key}.title': PRIVACY.map((k) => `privacy.${k}.title`),
  'privacy.${key}.body': PRIVACY.map((k) => `privacy.${k}.body`),
};

test('toda plantilla dinámica del código está en DINAMICAS, y ninguna de más', () => {
  expect(plantillasDinamicas(codigo)).toEqual(
    Object.keys(DINAMICAS).sort((a, b) => a.localeCompare(b)),
  );
});

test('toda clave que produce una plantilla dinámica existe en el catálogo', () => {
  const inexistentes = Object.values(DINAMICAS)
    .flat()
    .filter((k) => !(k in en));
  expect(inexistentes).toEqual([]);
});

test('ninguna clave del catálogo se queda sin usar', () => {
  expect(clavesSinUsar(Object.keys(en), codigo, DINAMICAS)).toEqual([]);
});
