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

// El corazón, con el dedo: solo el de las favoritas (relleno). El de las demás no se
// pinta ni se puede pulsar (`pointer-coarse:hidden`): era ruido en cada tesela y le
// robaba el toque a la foto. Igual en las apps nativas; se marca desde el visor o la
// barra de selección. Con ratón se sigue viendo al pasar o enfocar.
test('con el dedo solo se ve el corazón de las favoritas', () => {
  const corazon = (favorite: boolean): HTMLButtonElement => {
    montar(
      <Miniatura foto={{ ...foto, favorite }} t={t} onOpen={() => {}} onFavorite={() => {}} />,
    );
    const b = host.querySelector<HTMLButtonElement>('button[aria-pressed]')!;
    expect(b).not.toBeNull();
    return b;
  };
  const hueco = corazon(false);
  expect(hueco.className).toContain('pointer-coarse:hidden');
  expect(hueco.className).not.toContain('pointer-coarse:opacity-100');
  expect(hueco.className).toContain('group-hover:opacity-100');
  render(null, host);
  host.remove();
  const lleno = corazon(true);
  expect(lleno.className).not.toContain('pointer-coarse:hidden');
  expect(lleno.className).not.toContain('opacity-0');
  expect(lleno.querySelector('svg')?.getAttribute('fill')).toBe('currentColor');
});
