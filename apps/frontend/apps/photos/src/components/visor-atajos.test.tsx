/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja de atajos del visor: «?» y el botón «Atajos» la abren DENTRO del
 * `<dialog>` (fuera, en `body`, quedaría detrás de la capa superior), cuenta solo
 * las teclas que ese visor atiende y, mientras está abierta, las flechas no pasan
 * de foto por debajo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useT as traductor } from '../i18n/index.js';
import type { Foto } from '../client/photosApi.js';
import { Visor } from './PhotosApp.js';

// El DOM falso solo mientras corre ESTE fichero (ver miniatura-seleccion.test.tsx).
let registrado = false;
let fetchOriginal: typeof fetch;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registrado = true;
  }
  // El visor pide la receta y los álbumes; aquí no hay servidor.
  fetchOriginal = globalThis.fetch;
  globalThis.fetch = (() => new Promise<Response>(() => {})) as unknown as typeof fetch;
});
afterAll(async () => {
  globalThis.fetch = fetchOriginal;
  if (registrado) await GlobalRegistrator.unregister();
});

const t = traductor('es');
const foto = {
  id: 'f1',
  name: 'playa.jpg',
  width: 400,
  height: 300,
  sizeBytes: 1000,
  favorite: false,
  takenAt: '2026-09-10T12:00:00Z',
  takenAtFromExif: true,
} as unknown as Foto;

let host: HTMLElement;
afterEach(() => {
  act(() => render(null, host));
  host.remove();
});

interface Llamadas {
  prev: number;
  next: number;
  borradas: number;
}

function montar(conBorrar: boolean): { d: HTMLDialogElement; llamadas: Llamadas } {
  const llamadas: Llamadas = { prev: 0, next: 0, borradas: 0 };
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() =>
    render(
      <Visor
        foto={foto}
        t={t}
        locale="es"
        onClose={() => {}}
        onPrev={() => llamadas.prev++}
        onNext={() => llamadas.next++}
        {...(conBorrar ? { onDelete: () => llamadas.borradas++ } : {})}
      />,
      host,
    ),
  );
  return { d: host.querySelector('dialog')!, llamadas };
}

const tecla = (el: Element, key: string) =>
  act(() => {
    el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  });

/** La hoja abierta, si la hay: el `role=dialog` que pinta `ShortcutsModal`. */
const hoja = (d: HTMLDialogElement) =>
  [...d.querySelectorAll('[role="dialog"]')].find((n) =>
    n.textContent?.includes(t('shortcuts.title')),
  ) ?? null;

test('«?» abre la hoja dentro del visor, y con ella abierta las flechas no pasan de foto', () => {
  const { d, llamadas } = montar(true);
  expect(hoja(d)).toBeNull();
  tecla(d, '?');
  const h = hoja(d);
  expect(h).not.toBeNull();
  expect(h!.textContent).toContain(t('shortcuts.group.viewer'));
  expect(h!.textContent).toContain(t('viewer.delete'));
  // Sin lista que recorrer (no hay `onIr`), Inicio y Fin no se prometen.
  expect(h!.textContent).not.toContain(t('shortcuts.first'));
  tecla(h!, 'ArrowRight');
  expect(llamadas.next).toBe(0);
});

test('cada tecla de la hoja hace lo que dice', () => {
  const { d, llamadas } = montar(true);
  tecla(d, 'ArrowLeft');
  tecla(d, 'ArrowRight');
  expect(llamadas).toEqual({ prev: 1, next: 1, borradas: 0 });
  const ficha = () =>
    d.querySelector<HTMLButtonElement>(`button[aria-label="${t('viewer.info')}"]`)!;
  expect(ficha().getAttribute('aria-pressed')).toBe('false');
  tecla(d, 'i');
  expect(ficha().getAttribute('aria-pressed')).toBe('true');
  expect(d.querySelector(`button[aria-label="${t('viewer.zoomIn')}"]`)).not.toBeNull();
  tecla(d, 'z');
  expect(d.querySelector(`button[aria-label="${t('viewer.zoomOut')}"]`)).not.toBeNull();
  tecla(d, ' ');
  expect(d.querySelector(`button[aria-label="${t('viewer.slideshowStop')}"]`)).not.toBeNull();
  tecla(d, 'Delete');
  expect(llamadas.borradas).toBe(1);
});

test('el botón «Atajos» abre la misma hoja; sin papelera, no la cuenta', () => {
  const { d } = montar(false);
  const boton = d.querySelector<HTMLButtonElement>(`button[aria-label="${t('shortcuts.help')}"]`);
  expect(boton).not.toBeNull();
  expect(boton!.getAttribute('aria-keyshortcuts')).toBe('?');
  act(() => boton!.click());
  const h = hoja(d);
  expect(h).not.toBeNull();
  expect(h!.textContent).not.toContain(t('viewer.delete'));
  expect(h!.textContent).toContain(t('shortcuts.info'));
});
