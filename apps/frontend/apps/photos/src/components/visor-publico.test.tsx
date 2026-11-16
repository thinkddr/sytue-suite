/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La página pública de un álbum (`/p/{token}`): pulsar una tesela abre el visor con su
 * contador, sus flechas y «Descargar», en vez de llevar a la imagen cruda. Un clic con
 * Ctrl o ⌘ sigue siendo del navegador (abrir en otra pestaña).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useT as traductor } from '../i18n/index.js';
import { nombreDeDescarga } from './VisorCompartido.js';
import { VisorPublico, type FotoPublica } from './VisorPublico.js';

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
const fotos: FotoPublica[] = Array.from({ length: 12 }, (_, i) => ({
  id: `f${i}`,
  name: `IMG_${1000 + i}.jpg`,
  width: 400,
  height: 300,
  takenAt: '2026-09-10T12:00:00Z',
  grande: `/p/tok/img/f${i}/grande`,
}));

let pagina: HTMLElement;
afterEach(() => {
  act(() => render(null, pagina.querySelector('#isla')!));
  pagina.remove();
});

/** Lo que pinta el servidor —las teselas enlazadas— y la isla al lado. */
function montar(): HTMLElement {
  pagina = document.createElement('div');
  pagina.innerHTML =
    '<ul>' +
    fotos
      .map((f, i) => `<li><a href="${f.grande}" data-visor="${i}"><img alt="${f.name}"></a></li>`)
      .join('') +
    '</ul><div id="isla"></div>';
  document.body.appendChild(pagina);
  act(() => render(<VisorPublico fotos={fotos} locale="es" />, pagina.querySelector('#isla')!));
  return pagina;
}

function pulsar(i: number, extra: MouseEventInit = {}): MouseEvent {
  const e = new MouseEvent('click', { bubbles: true, cancelable: true, button: 0, ...extra });
  act(() => {
    pagina.querySelector(`a[data-visor="${i}"] img`)!.dispatchEvent(e);
  });
  return e;
}
const contador = () =>
  pagina.querySelector('dialog header [aria-hidden="true"]')?.textContent ?? null;

test('sin pulsar nada no hay visor', () => {
  montar();
  expect(pagina.querySelector('dialog')).toBeNull();
});

test('pulsar la cuarta foto abre el visor en «4 / 12» y no navega', () => {
  montar();
  const e = pulsar(3);
  expect(e.defaultPrevented).toBe(true);
  expect(contador()).toBe(t('viewer.counter', { index: 4, total: 12 }));
  expect(pagina.querySelector('dialog img')?.getAttribute('src')).toBe(fotos[3]!.grande);
});

test('las flechas, Inicio y Fin recorren el álbum', () => {
  montar();
  pulsar(3);
  const d = pagina.querySelector('dialog')!;
  const tecla = (key: string) =>
    act(() => {
      d.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
    });
  tecla('ArrowRight');
  expect(contador()).toBe(t('viewer.counter', { index: 5, total: 12 }));
  tecla('End');
  expect(contador()).toBe(t('viewer.counter', { index: 12, total: 12 }));
  tecla('Home');
  expect(contador()).toBe(t('viewer.counter', { index: 1, total: 12 }));
  expect(d.querySelector(`[aria-label="${t('viewer.prev')}"]`)).toBeNull();
  expect(d.querySelector(`[aria-label="${t('viewer.next')}"]`)).not.toBeNull();
});

test('«Descargar» es un enlace a la variante grande', () => {
  montar();
  pulsar(2);
  const a = pagina.querySelector<HTMLAnchorElement>(
    `dialog a[aria-label="${t('viewer.download')}"]`,
  );
  expect(a?.getAttribute('href')).toBe(fotos[2]!.grande);
  expect(a?.getAttribute('download')).toBe(fotos[2]!.name);
});

test('la descarga se guarda con la extensión de la variante grande, que es JPEG', () => {
  expect(nombreDeDescarga('IMG_1.HEIC')).toBe('IMG_1.jpg');
  expect(nombreDeDescarga('playa.de.verano.png')).toBe('playa.de.verano.jpg');
  expect(nombreDeDescarga('sin-extension')).toBe('sin-extension.jpg');
  expect(nombreDeDescarga('.oculto')).toBe('.oculto.jpg');
});

test('al cerrar, el foco vuelve a la tesela de la foto vista, ya sin el diálogo', () => {
  montar();
  pulsar(3);
  const d = pagina.querySelector('dialog')!;
  act(() => {
    d.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
  });
  // En un navegador, mientras el diálogo modal sigue abierto la página es inerte y
  // `focus()` no hace nada: el foco tiene que llegar cuando el diálogo ya no está.
  const tesela = pagina.querySelector<HTMLAnchorElement>('a[data-visor="4"]')!;
  let conDialogo = null as boolean | null;
  tesela.addEventListener('focus', () => {
    conDialogo = pagina.querySelector('dialog') !== null;
  });
  act(() => {
    pagina.querySelector<HTMLButtonElement>(`dialog [aria-label="${t('viewer.close')}"]`)!.click();
  });
  expect(conDialogo).toBe(false);
  expect(document.activeElement).toBe(tesela);
});

test('cerrar quita el visor', () => {
  montar();
  pulsar(0);
  act(() => {
    pagina.querySelector<HTMLButtonElement>(`dialog [aria-label="${t('viewer.close')}"]`)!.click();
  });
  expect(pagina.querySelector('dialog')).toBeNull();
});

test('con Ctrl o ⌘ el clic sigue siendo del navegador', () => {
  montar();
  expect(pulsar(1, { ctrlKey: true }).defaultPrevented).toBe(false);
  expect(pulsar(1, { metaKey: true }).defaultPrevented).toBe(false);
  expect(pagina.querySelector('dialog')).toBeNull();
});
