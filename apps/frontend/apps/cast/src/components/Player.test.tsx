/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El menú de calidad del reproductor (sobre un hls.js de mentira: el de verdad
 * necesita MediaSource), el de velocidad con sus teclas < y >, y la tecla P de imagen
 * dentro de imagen.
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
const { I18nProvider } = await import('@sytue/i18n/preact');
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});

async function montar(fuente: 'hls' | 'webm' = 'hls'): Promise<HTMLElement> {
  host = document.createElement('div');
  document.body.appendChild(host);
  // happy-dom no reproduce HLS: así el reproductor va por hls.js.
  HTMLMediaElement.prototype.canPlayType = () => '';
  creados.length = 0;
  await act(() => {
    render(
      // El idioma de la página: la velocidad se escribe con su separador decimal.
      <I18nProvider locale="es">
        {fuente === 'hls' ? (
          <Player t={t} hls="/media/v/t/hls/master.m3u8" titulo="Prueba" />
        ) : (
          <Player t={t} hls={null} webm="/media/v/t/video.webm" titulo="Prueba" />
        )}
      </I18nProvider>,
      host,
    );
  });
  // hls.js se carga a demanda (import dinámico): se espera a que el reproductor lo cree.
  for (let i = 0; fuente === 'hls' && i < 100 && !ultimo(); i++)
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

const botonVelocidad = (h: HTMLElement): HTMLButtonElement =>
  h.querySelector<HTMLButtonElement>('[aria-label^="Velocidad:"]')!;
const pulsar = async (h: HTMLElement, key: string): Promise<void> => {
  await act(() => {
    h.querySelector('[role="region"]')!.dispatchEvent(
      new KeyboardEvent('keydown', { key, bubbles: true }),
    );
  });
};

test('el menú de velocidad va de 0,5× a 2×, cambia el vídeo y lo anuncia', async () => {
  const h = await montar();
  const v = h.querySelector('video')!;
  const boton = botonVelocidad(h);
  expect(boton.textContent).toBe('1×');
  expect(boton.getAttribute('aria-label')).toBe('Velocidad: 1×');
  expect(h.querySelector('[role="status"]')?.textContent).toBe(''); // al cargar, nada
  await act(() => boton.click());
  const menu = h.querySelector('[role="menu"][aria-label="Velocidad"]')!;
  const opciones = Array.from(menu.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]'));
  expect(opciones.map((o) => o.textContent)).toEqual([
    '0,5×',
    '0,75×',
    'Normal',
    '1,25×',
    '1,5×',
    '1,75×',
    '2×',
  ]);
  expect(opciones[2]!.getAttribute('aria-checked')).toBe('true');
  opciones[4]!.focus();
  await act(() => opciones[4]!.click());
  expect(v.playbackRate).toBe(1.5);
  // El foco vuelve al botón, no al `body`: las teclas del reproductor siguen.
  expect(document.activeElement).toBe(botonVelocidad(h));
  // También la por defecto: un `load()` (otra fuente, recuperarse) no la devuelve a 1×.
  expect(v.defaultPlaybackRate).toBe(1.5);
  expect(h.querySelector('[role="menu"]')).toBeNull();
  expect(botonVelocidad(h).textContent).toBe('1,5×');
  expect(h.querySelector('[role="status"]')?.textContent).toBe('Velocidad 1,5×');
  await act(() => botonVelocidad(h).click());
  expect(h.querySelector('[role="menuitemradio"][aria-checked="true"]')?.textContent).toBe('1,5×');
  // Escape lo cierra, como el de calidad, y devuelve el foco al botón.
  h.querySelector<HTMLButtonElement>('[role="menuitemradio"]')!.focus();
  await pulsar(h, 'Escape');
  expect(h.querySelector('[role="menu"]')).toBeNull();
  expect(document.activeElement).toBe(botonVelocidad(h));
});

test('< y > bajan y suben la velocidad un peldaño, sin dar la vuelta', async () => {
  const h = await montar();
  const v = h.querySelector('video')!;
  await pulsar(h, '>');
  await pulsar(h, '>');
  expect(v.playbackRate).toBe(1.5);
  expect(h.querySelector('[role="status"]')?.textContent).toBe('Velocidad 1,5×');
  await pulsar(h, '<');
  expect(v.playbackRate).toBe(1.25);
  for (let i = 0; i < 6; i++) await pulsar(h, '>');
  expect(v.playbackRate).toBe(2);
  for (let i = 0; i < 9; i++) await pulsar(h, '<');
  expect(v.playbackRate).toBe(0.5);
  expect(botonVelocidad(h).textContent).toBe('0,5×');
});

test('la velocidad está también sin hls.js (el WebM de respaldo), donde no hay calidad', async () => {
  const h = await montar('webm');
  expect(h.querySelector('[aria-label="Calidad"]')).toBeNull();
  expect(botonVelocidad(h)).not.toBeNull();
  await pulsar(h, '>');
  expect(h.querySelector('video')!.playbackRate).toBe(1.25);
});

test('abrir un menú cierra el otro', async () => {
  const h = await montar();
  await act(() => ultimo()!.emitir('manifest'));
  await act(() => botonVelocidad(h).click());
  await act(() => h.querySelector<HTMLButtonElement>('[aria-label="Calidad"]')!.click());
  expect(h.querySelectorAll('[role="menu"]').length).toBe(1);
  expect(h.querySelector('[role="menu"]')!.getAttribute('aria-label')).toBe('Calidad');
  await act(() => botonVelocidad(h).click());
  expect(h.querySelectorAll('[role="menu"]').length).toBe(1);
  expect(h.querySelector('[role="menu"]')!.getAttribute('aria-label')).toBe('Velocidad');
});

test('con un menú abierto la barra no se esconde aunque el vídeo siga', async () => {
  const h = await montar();
  const v = h.querySelector('video')!;
  // happy-dom no reproduce: se finge que el vídeo va.
  Object.defineProperty(v, 'paused', { configurable: true, get: () => false });
  // `play` arranca el temporizador que esconde la barra.
  await act(() => {
    v.dispatchEvent(new Event('play'));
  });
  await act(() => botonVelocidad(h).click());
  await act(async () => {
    await new Promise((r) => setTimeout(r, 2700));
  });
  const barra = botonVelocidad(h).closest('.transition-opacity')!;
  expect(barra.className).toContain('opacity-100');
  expect(h.querySelector('[role="menu"]')).not.toBeNull();
});

test('J y L mueven 10 s, Inicio y Fin van a los extremos y 0–9 a las décimas', async () => {
  const h = await montar();
  const v = h.querySelector('video')!;
  // happy-dom no carga vídeo: una duración de mentira.
  Object.defineProperty(v, 'duration', { configurable: true, get: () => 200 });
  v.currentTime = 50;
  await pulsar(h, 'l');
  expect(v.currentTime).toBe(60);
  await pulsar(h, 'J');
  await pulsar(h, 'j');
  expect(v.currentTime).toBe(40);
  await pulsar(h, '7');
  expect(v.currentTime).toBe(140);
  await pulsar(h, 'End');
  expect(v.currentTime).toBe(200);
  await pulsar(h, 'Home');
  expect(v.currentTime).toBe(0);
  // Con Ctrl, el número es del navegador (cambiar de pestaña), no del vídeo.
  await act(() => {
    h.querySelector('[role="region"]')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: '5', ctrlKey: true, bubbles: true }),
    );
  });
  expect(v.currentTime).toBe(0);
});

test('«Repetir», en el menú de la velocidad, pone y quita el `loop` del vídeo', async () => {
  const h = await montar();
  const v = h.querySelector('video')!;
  expect(v.loop).toBe(false);
  await act(() => botonVelocidad(h).click());
  const repetir = h.querySelector<HTMLButtonElement>('[role="menuitemcheckbox"]')!;
  expect(repetir.textContent).toBe('Repetir');
  expect(repetir.getAttribute('aria-checked')).toBe('false');
  await act(() => repetir.click());
  expect(v.loop).toBe(true);
  expect(h.querySelector('[role="menu"]')).toBeNull();
  expect(document.activeElement).toBe(botonVelocidad(h));
  await act(() => botonVelocidad(h).click());
  expect(h.querySelector('[role="menuitemcheckbox"]')?.getAttribute('aria-checked')).toBe('true');
});
