/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ENLACE DE UNA REUNIÓN ES `https://rooms.<dominio>/?rooms=<id>` Y NADA LO ESCRIBE A MANO.
 *
 * Lo decidió el dueño el 2026-09-25, después de que un enlace de Calendar
 * (`rooms.sytue.eu/?meet=<id>`) acabara tras el login en `corporate.sytue.eu/?meet=<id>`:
 * el Hub en vez de la reunión. El enlace lo generaban cuatro sitios (Calendar, «Copiar
 * el enlace» de Rooms, Rooms y Calendar nativos), cada uno con su host y su prefijo.
 * Ahora sale de `enlaceReunion` en la web y de `api.EnlaceReunion` en Go.
 *
 * El parámetro anterior, `meet`, solo se LEE (alias en `pages/index.astro` de Rooms y
 * al pegar un enlace). Este guardián mira el código fuente de `apps/` y `services/` y
 * falla si alguien vuelve a componerlo. Los tests quedan fuera: prueban el alias.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { enlaceReunion } from './tokens.js';

const REPO = fileURLToPath(new URL('../../../../../', import.meta.url));
const FUERA = new Set(['node_modules', 'dist', '.astro', '.git', 'vendor', 'testdata', 'build']);
const FUENTE = /\.(ts|tsx|astro|go|json|tmpl|html)$/;
const PRUEBA = /(_test\.go|\.test\.tsx?|\.spec\.ts)$/;
// Componer el parámetro viejo: en una URL o poniéndolo en unos URLSearchParams.
const GENERA_MEET = /[?&]meet=|\.set\(\s*['"]meet['"]|\.Set\("meet"/;

function ficheros(dir: string, out: string[] = []): string[] {
  for (const e of readdirSync(dir)) {
    if (FUERA.has(e)) continue;
    const p = join(dir, e);
    if (statSync(p).isDirectory()) ficheros(p, out);
    else if (FUENTE.test(e) && !PRUEBA.test(e)) out.push(p);
  }
  return out;
}

test('enlaceReunion: host propio de Rooms, en la raíz, con ?rooms=', () => {
  expect(enlaceReunion('a b', '', 'https://rooms.sytue.eu/')).toBe(
    'https://rooms.sytue.eu/?rooms=a%20b',
  );
  expect(enlaceReunion('x', '')).toMatch(/^https:\/\/rooms\.[^/]+\/\?rooms=x$/);
});

test('enlaceReunion: la clave de la reunión suelta viaja en el enlace', () => {
  expect(enlaceReunion('x', 'k1', 'https://rooms.sytue.eu')).toBe(
    'https://rooms.sytue.eu/?rooms=x&clave=k1',
  );
});

test('nada en apps/ ni services/ genera ya un enlace con ?meet=', () => {
  const malos: string[] = [];
  for (const raiz of ['apps', 'services']) {
    for (const f of ficheros(join(REPO, raiz))) {
      if (GENERA_MEET.test(readFileSync(f, 'utf8'))) malos.push(f.slice(REPO.length));
    }
  }
  expect(malos).toEqual([]);
});
