/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Guardián de seguridad: toda salida del renderizador en la app debe ir en un
 * iframe aislado con sandbox, inert y tabindex=-1; nunca mediante
 * dangerouslySetInnerHTML. PAGES-PLAN §14.
 */
import { leerFuentes } from '@sytue/config/guardianes';
import { expect, test } from 'bun:test';

test('ningún fichero de src/ usa dangerouslySetInnerHTML', async () => {
  // Con el recorrido a cero, este guardián seguía en verde (medido el 2026-11-25, ver
  // `@sytue/config/guardianes`): de 208 ficheros, que salgan 100 como poco.
  const fuentes = await leerFuentes({
    glob: '**/*',
    cwd: new URL('./', import.meta.url),
    minimo: 100,
    filtro: (rel) => /\.(ts|tsx|astro)$/.test(rel) && !rel.includes('.test.'),
  });
  const infractores = fuentes
    .filter(([, fuente]) => fuente.includes('dangerouslySetInnerHTML'))
    .map(([rel]) => rel);
  expect(infractores).toEqual([]);
});
