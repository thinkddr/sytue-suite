/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { LOCALES } from '@sytue/i18n';
import { en } from './messages/en.js';

const DIR = join(import.meta.dir, 'messages');
const idiomas = readdirSync(DIR)
  .filter((f) => /^[a-z]{2}\.ts$/.test(f))
  .map((f) => f.slice(0, 2))
  .sort();

// Los 26 idiomas de la suite están publicados: un catálogo que no esté aquí, o uno de
// aquí que falte, pone el test en rojo.
const PUBLICADOS = [...LOCALES].sort();

test('los catálogos son exactamente los idiomas publicados, todos de la suite', () => {
  expect(idiomas).toEqual(PUBLICADOS);
  expect(PUBLICADOS.filter((l) => !(LOCALES as readonly string[]).includes(l))).toEqual([]);
});

test('index.ts importa los idiomas publicados, ni uno más ni uno menos', () => {
  const fuente = readFileSync(join(import.meta.dir, 'index.ts'), 'utf8');
  const importados = [...fuente.matchAll(/import \{ ([a-z]{2}) \} from '\.\/messages\//g)]
    .map((m) => m[1] ?? '')
    .sort((a, b) => a.localeCompare(b));
  expect(importados).toEqual(PUBLICADOS);
});

const textos = (v: unknown): string[] =>
  typeof v === 'string' ? [v] : Object.values(v as Record<string, string>);
const parametros = (v: unknown) =>
  [
    ...new Set(textos(v).flatMap((m) => [...m.matchAll(/\{(\w+)\}/g)].map((x) => x[1] ?? ''))),
  ].sort();

// El tipo ya obliga a tener las mismas claves; esto mira dentro de cada texto. Un
// `{count}` puede faltar en una forma plural que no lo necesita, pero ningún idioma
// pide un parámetro que el inglés no pase.
test.each(idiomas)('%s: mismas claves y ningún parámetro que el inglés no tenga', async (l) => {
  const cat = ((await import(join(DIR, `${l}.ts`))) as Record<string, Record<string, unknown>>)[l]!;
  expect(Object.keys(cat).sort()).toEqual(Object.keys(en).sort());
  const ajenos = Object.keys(en).filter((k) => {
    const suyos = parametros(cat[k]);
    const delIngles = parametros((en as Record<string, unknown>)[k]);
    return (
      suyos.some((p) => !delIngles.includes(p)) ||
      (typeof cat[k] === 'string' && delIngles.some((p) => !suyos.includes(p)))
    );
  });
  expect(ajenos).toEqual([]);
});
