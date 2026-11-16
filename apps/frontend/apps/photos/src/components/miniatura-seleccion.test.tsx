/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tesela de la galería y la selección: Mayús + clic le dice a la galería que marque
 * el TRAMO, y mantenerla pulsada con el dedo entra en el modo selección con esa foto
 * marcada, sin abrir el visor.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { act } from 'preact/test-utils';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import type { Foto } from '../client/photosApi.js';
import { Miniatura } from './PhotosApp.js';

// El DOM falso solo mientras corre ESTE fichero: Bun corre todos los de la app en el
// mismo proceso, y con `window` registrado los que montan su propio `localStorage` o
// miran si hay navegador (fondoLienzo, abrir-desde-otro-sitio) dejaban de valer.
let registrado = false;
beforeAll(() => {
  if (typeof document !== 'undefined') return;
  GlobalRegistrator.register();
  registrado = true;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = traductor('es');
const foto = {
  id: 'f1',
  name: 'playa.jpg',
  width: 400,
  height: 300,
  favorite: false,
  takenAtFromExif: true,
} as unknown as Foto;

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});
function montar(v: VNode): HTMLButtonElement {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(v, host));
  return host.querySelector('button')!;
}
const dedo = (tipo: string) =>
  new PointerEvent(tipo, {
    pointerId: 1,
    pointerType: 'touch',
    isPrimary: true,
    clientX: 10,
    clientY: 10,
    bubbles: true,
    cancelable: true,
  });

test('en modo selección, el clic dice si llevaba Mayús', () => {
  const clics: boolean[] = [];
  const b = montar(
    <Miniatura
      foto={foto}
      t={t}
      onOpen={() => clics.push(false)}
      seleccionada={false}
      onToggle={(mayus) => clics.push(mayus)}
    />,
  );
  b.dispatchEvent(new MouseEvent('click', { bubbles: true, shiftKey: true }));
  b.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  expect(clics).toEqual([true, false]);
  // Mayús + pulsar no arrastra la selección de texto del navegador por la galería.
  const abajo = new MouseEvent('mousedown', { bubbles: true, cancelable: true, shiftKey: true });
  b.dispatchEvent(abajo);
  expect(abajo.defaultPrevented).toBe(true);
});

test('mantener pulsada la tesela empieza a seleccionar sin abrir el visor', async () => {
  let abiertas = 0;
  let mantenidas = 0;
  const b = montar(
    <Miniatura foto={foto} t={t} onOpen={() => abiertas++} onMantener={() => mantenidas++} />,
  );
  b.dispatchEvent(dedo('pointerdown'));
  await new Promise((r) => setTimeout(r, 480));
  b.dispatchEvent(dedo('pointerup'));
  b.dispatchEvent(new MouseEvent('click', { detail: 1, bubbles: true, cancelable: true }));
  expect(mantenidas).toBe(1);
  expect(abiertas).toBe(0);
  // Un toque corto sigue abriendo la foto.
  b.dispatchEvent(dedo('pointerdown'));
  b.dispatchEvent(dedo('pointerup'));
  b.dispatchEvent(new MouseEvent('click', { detail: 1, bubbles: true, cancelable: true }));
  expect(abiertas).toBe(1);
  expect(mantenidas).toBe(1);
});
