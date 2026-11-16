/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tarjeta de álbum dice si el álbum tiene enlace público, y «Ordenar por» ofrece
 * los tres órdenes con su rótulo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { act } from 'preact/test-utils';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import type { Album } from '../client/photosApi.js';
import type { OrdenAlbum } from '../lib/ordenAlbumes.js';
import { SelectorOrdenAlbumes, TarjetaAlbum } from './Albums.js';

let registrado = false;
beforeAll(() => {
  if (typeof document !== 'undefined') return;
  GlobalRegistrator.register();
  registrado = true;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});
function montar(nodo: ComponentChild): HTMLElement {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(nodo, host));
  return host;
}

const album: Album = {
  id: 'a1',
  title: 'Verano',
  coverAssetId: null,
  count: 3,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-02T00:00:00Z',
};

test('un álbum con enlace público lleva la insignia «Compartido»', () => {
  const h = montar(<TarjetaAlbum t={traductor('es')} album={{ ...album, shared: true }} />);
  const enlace = h.querySelector('a');
  expect(enlace?.textContent).toContain('Compartido');
  expect(enlace?.textContent).toContain('Verano');
  expect(enlace?.textContent).toContain('3 fotos');
});

test('un álbum sin enlace no la lleva', () => {
  const h = montar(<TarjetaAlbum t={traductor('es')} album={album} />);
  expect(h.textContent).not.toContain('Compartido');
});

test('«Ordenar por» nombra el campo y avisa del orden elegido', () => {
  let elegido: OrdenAlbum | null = null;
  const h = montar(
    <SelectorOrdenAlbumes t={traductor('es')} orden="recent" alCambiar={(o) => (elegido = o)} />,
  );
  const select = h.querySelector('select') as HTMLSelectElement;
  expect(select.closest('label')?.textContent).toContain('Ordenar por');
  expect([...select.options].map((o) => o.textContent)).toEqual([
    'Modificados recientemente',
    'Nombre',
    'Más fotos',
  ]);
  act(() => {
    select.value = 'count';
    select.dispatchEvent(new Event('change', { bubbles: true }));
  });
  expect(elegido as OrdenAlbum | null).toBe('count');
});
