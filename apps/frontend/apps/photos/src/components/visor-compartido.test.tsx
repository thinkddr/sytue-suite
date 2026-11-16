/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El visor de un álbum compartido conmigo dice dónde va («5 / 30», y «Foto 5 de 30»
 * para el lector) y salta con Inicio y Fin, como el de la galería. Una foto sola no
 * lleva contador.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import type { FotoCompartida } from '../client/photosApi.js';
import { posicionVisor } from '../lib/posicionVisor.js';
import { VisorCompartido } from './Compartidos.js';

// El DOM falso solo mientras corre ESTE fichero (ver miniatura-seleccion.test.tsx).
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
const fotos: FotoCompartida[] = Array.from({ length: 30 }, (_, i) => ({
  id: `f${i}`,
  name: `IMG_${1000 + i}.jpg`,
  width: 400,
  height: 300,
  takenAt: '2026-09-10T12:00:00Z',
}));

/** Lo que hace `CompartidoConmigo` con el visor: la foto abierta es un índice. */
function Album({ items, desde }: { items: FotoCompartida[]; desde: number }) {
  const [i, setI] = useState(desde);
  return (
    <VisorCompartido
      src="/api/v1/shared/albums/a1/grande"
      foto={items[i]!}
      t={t}
      onClose={() => {}}
      posicion={posicionVisor(i, items.length)}
      onIr={setI}
      onPrev={i > 0 ? () => setI(i - 1) : undefined}
      onNext={i < items.length - 1 ? () => setI(i + 1) : undefined}
    />
  );
}

let host: HTMLElement;
afterEach(() => {
  act(() => render(null, host));
  host.remove();
});
function montar(items: FotoCompartida[], desde: number): HTMLDialogElement {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(<Album items={items} desde={desde} />, host));
  return host.querySelector('dialog')!;
}
const tecla = (d: HTMLDialogElement, key: string) =>
  act(() => {
    d.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  });
const cabecera = (d: HTMLDialogElement) => d.querySelector('header')!.textContent ?? '';

test('la quinta de 30 dice «5 / 30», con su frase para el lector', () => {
  const d = montar(fotos, 4);
  const visible = d.querySelector('header [aria-hidden="true"]');
  expect(visible?.textContent).toBe(t('viewer.counter', { index: 5, total: 30 }));
  expect(d.querySelector('header .sr-only')?.textContent).toBe(
    t('viewer.counterLabel', { index: 5, total: 30 }),
  );
});

test('Fin lleva a la última e Inicio a la primera', () => {
  const d = montar(fotos, 4);
  tecla(d, 'End');
  expect(cabecera(d)).toContain('IMG_1029.jpg');
  expect(cabecera(d)).toContain(t('viewer.counter', { index: 30, total: 30 }));
  tecla(d, 'Home');
  expect(cabecera(d)).toContain('IMG_1000.jpg');
  // Las flechas siguen pasando de una en una.
  tecla(d, 'ArrowRight');
  expect(cabecera(d)).toContain('IMG_1001.jpg');
});

test('una foto sola no lleva contador', () => {
  const d = montar(fotos.slice(0, 1), 0);
  expect(d.querySelector('header .tabular-nums')).toBeNull();
});

test('el escenario lleva el fondo del visor, no un negro suelto', () => {
  const d = montar(fotos, 0);
  expect(d.innerHTML).not.toContain('bg-[#0e0e0e]');
});
