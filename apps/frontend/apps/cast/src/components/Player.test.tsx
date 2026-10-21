/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El menú de calidad del reproductor (sobre un hls.js de mentira: el de verdad
 * necesita MediaSource) y la tecla P de imagen dentro de imagen.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, mock, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';

if (typeof document === 'undefined') GlobalRegistrator.register();

type Oyente = (e: string, datos: { level: number }) => void;
const creados: HlsFalso[] = [];
const ultimo = (): HlsFalso | undefined => creados.at(-1);
class HlsFalso {
  static isSupported = (): boolean => true;
  static Events = { ERROR: 'error', MANIFEST_PARSED: 'manifest', LEVEL_SWITCHED: 'switched' };
  static ErrorTypes = { MEDIA_ERROR: 'media' };
  levels = [{ height: 360 }, { height: 1080 }, { height: 720 }];
  currentLevel = -1;
  oyentes = new Map<string, Oyente>();
  constructor() {
    creados.push(this);
  }
  on(e: string, f: Oyente): void {
    this.oyentes.set(e, f);
  }
  emitir(e: string, datos = { level: 0 }): void {
    this.oyentes.get(e)?.(e, datos);
  }
  loadSource(): void {}
  attachMedia(): void {}
  destroy(): void {}
  recoverMediaError(): void {}
}
void mock.module('hls.js', () => ({ default: HlsFalso }));

const { Player } = await import('./Player.js');
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});

async function montar(): Promise<HTMLElement> {
  host = document.createElement('div');
  document.body.appendChild(host);
  // happy-dom no reproduce HLS: así el reproductor va por hls.js.
  HTMLMediaElement.prototype.canPlayType = () => '';
  creados.length = 0;
  await act(() => {
    render(<Player t={t} hls="/media/v/t/hls/master.m3u8" titulo="Prueba" />, host);
  });
  // hls.js se carga a demanda (import dinámico): se espera a que el reproductor lo cree.
  for (let i = 0; i < 100 && !ultimo(); i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 10));
    });
  return host;
}

test('el menú de calidad ofrece Automática y las del master, y elegir una la fija', async () => {
  const h = await montar();
  expect(h.querySelector('[aria-label="Calidad"]')).toBeNull(); // sin manifiesto, sin menú
  await act(() => ultimo()!.emitir('manifest'));
  const boton = h.querySelector<HTMLButtonElement>('[aria-label="Calidad"]')!;
  expect(boton).not.toBeNull();
  await act(() => boton.click());
  const opciones = Array.from(h.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]'));
  expect(opciones.map((o) => o.textContent)).toEqual(['Automática', '1080p', '720p', '360p']);
  expect(opciones[0]!.getAttribute('aria-checked')).toBe('true');
  // Elegir 360p fija el nivel de hls.js (su índice, no su posición en el menú).
  await act(() => opciones[3]!.click());
  expect(ultimo()!.currentLevel).toBe(0);
  await act(() => h.querySelector<HTMLButtonElement>('[aria-label="Calidad"]')!.click());
  expect(h.querySelector('[role="menuitemradio"][aria-checked="true"]')?.textContent).toBe('360p');
  // Automática vuelve a -1 y dice cuál suena.
  await act(() =>
    Array.from(h.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]'))[0]!.click(),
  );
  expect(ultimo()!.currentLevel).toBe(-1);
  await act(() => ultimo()!.emitir('switched', { level: 2 }));
  await act(() => h.querySelector<HTMLButtonElement>('[aria-label="Calidad"]')!.click());
  expect(h.querySelector('[role="menuitemradio"]')?.textContent).toBe('Automática (720p)');
});

test('P abre la imagen dentro de imagen, como el botón', async () => {
  let pedida = 0;
  Object.defineProperty(document, 'pictureInPictureEnabled', { value: true, configurable: true });
  HTMLVideoElement.prototype.requestPictureInPicture = () => {
    pedida++;
    return Promise.resolve({} as PictureInPictureWindow);
  };
  const h = await montar();
  const region = h.querySelector<HTMLElement>('[role="region"]')!;
  await act(() => {
    region.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', bubbles: true }));
  });
  expect(pedida).toBe(1);
});
