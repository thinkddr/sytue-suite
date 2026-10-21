/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Desarchivar» en la vista Archivo solo para el dueño: el servicio responde 404
 * a cualquier otro, así que el botón fallaría siempre.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { Notebook } from '@sytue/api-clients';
import { PanelArchivo } from './Archivo.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const nb = (id: string, owner_sub: string): Notebook =>
  ({ id, title: id, layout: 'list', owner_sub, archived_at: '2026-09-30T10:00:00Z' }) as Notebook;

test('el Archivo solo ofrece desarchivar los cuadernos propios', () => {
  const raiz = document.createElement('div');
  render(
    <PanelArchivo
      t={t}
      cuadernos={[nb('mio', 'ada'), nb('ajeno', 'bob')]}
      userId="ada"
      onAbrir={() => undefined}
      onDesarchivar={() => undefined}
    />,
    raiz,
  );
  const filas = [...raiz.querySelectorAll('li')].map((li) => li.textContent);
  expect(filas).toEqual(['mioarchive.unarchive', 'ajeno']);
});
