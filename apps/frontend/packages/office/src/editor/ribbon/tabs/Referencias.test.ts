/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Actualizar tabla» titulaba el índice con el LITERAL `{t('ref.toc')}`: a la
 * plantilla de `reconstruirTdc` le faltaba el `$`, y una plantilla sin `$` no es
 * un error de nadie —ni del compilador ni del linter—, es texto. Lo que se fija
 * aquí es lo que se veía: que el título sea el del catálogo y no la clave.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { docsT } from '../../../i18n';
import { reconstruirTdc } from './Referencias';

if (typeof document === 'undefined') GlobalRegistrator.register();

test('el índice reconstruido se titula con el texto traducido, no con la clave', () => {
  document.documentElement.lang = 'es';
  const root = document.createElement('div');
  root.innerHTML = '<nav data-toc><p><b>viejo</b></p><ol></ol></nav><h1>Uno</h1><h2>Dos</h2>';
  const nav = root.querySelector('nav[data-toc]');
  if (!nav) throw new Error('no hay índice');

  reconstruirTdc(root, nav);

  const titulo = nav.querySelector('p > b')?.textContent;
  expect(titulo).toBe(docsT('es')('ref.toc'));
  expect(titulo).toBe('Tabla de contenidos');
  expect(nav.innerHTML).not.toContain('ref.toc');
  expect(nav.querySelectorAll('li')).toHaveLength(2);
});
