/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Toda superficie SSR monta la página 404 de la suite. Sin la integración no
 * falla nada: Astro sirve la suya, en inglés y con su cohete, y nadie lo ve
 * hasta que alguien sigue un enlace roto en producción — que es como se midió.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';

const RAIZ_APPS = new URL('../../../apps/', import.meta.url);

test('cada app SSR inyecta la página 404 de la suite', async () => {
  const sin: string[] = [];
  let ssr = 0;
  for await (const rel of new Bun.Glob('*/astro.config.mjs').scan({ cwd: RAIZ_APPS.pathname })) {
    const fuente = await Bun.file(new URL(rel, RAIZ_APPS)).text();
    if (!/output:\s*'server'/.test(fuente)) continue;
    ssr++;
    if (!/integrations:\s*\[[^\]]*sytueNoEncontrada\(\)/.test(fuente)) sin.push(rel);
  }
  expect(ssr, 'no se encontró ninguna app SSR: ¿se movió apps/?').toBeGreaterThan(10);
  expect(sin, 'monta `sytueNoEncontrada()` de @sytue/config/astro-404').toEqual([]);
});
