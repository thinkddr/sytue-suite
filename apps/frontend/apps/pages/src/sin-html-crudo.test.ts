/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Guardián de seguridad: toda salida del renderizador en la app debe ir en un
 * iframe aislado con sandbox, inert y tabindex=-1; nunca mediante
 * dangerouslySetInnerHTML. PAGES-PLAN §14.
 */
import { expect, test } from 'bun:test';

const RAIZ_SRC = new URL('./', import.meta.url);

test('ningún fichero de src/ usa dangerouslySetInnerHTML', async () => {
  const infractores: string[] = [];
  for await (const rel of new Bun.Glob('**/*').scan({ cwd: RAIZ_SRC.pathname })) {
    if (!/\.(ts|tsx|astro)$/.test(rel) || rel.includes('.test.')) continue;
    const fuente = await Bun.file(new URL(rel, RAIZ_SRC)).text();
    if (fuente.includes('dangerouslySetInnerHTML')) {
      infractores.push(rel);
    }
  }
  expect(infractores).toEqual([]);
});
