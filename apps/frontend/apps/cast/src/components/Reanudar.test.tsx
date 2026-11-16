/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Seguir donde se dejó: el reproductor salta al punto guardado y ofrece volver al
 * principio, guarda mientras suena y al salir, y las tarjetas enseñan lo visto.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { Player } = await import('./Player.js');
const { VideoCard } = await import('./VideoCard.js');
const { I18nProvider } = await import('@sytue/i18n/preact');
const { desde, guardar, leer } = await import('../lib/reanudar.js');
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

let host: HTMLElement;
beforeEach(() => {
  localStorage.clear();
  host = document.createElement('div');
  document.body.appendChild(host);
});
afterEach(() => {
  render(null, host);
  host.remove();
});

/** Un `<video>` con lo que happy-dom no simula: duración, posición y estado de carga. */
function simular(v: HTMLVideoElement, duracion: number): { pos: () => number } {
  let pos = 0;
  let pausado = true;
  Object.defineProperty(v, 'duration', { configurable: true, get: () => duracion });
  Object.defineProperty(v, 'readyState', { configurable: true, get: () => 1 });
  Object.defineProperty(v, 'paused', { configurable: true, get: () => pausado });
  Object.defineProperty(v, 'currentTime', {
    configurable: true,
    get: () => pos,
    set: (s: number) => {
      pos = s;
    },
  });
  v.play = () => {
    pausado = false;
    return Promise.resolve();
  };
  return { pos: () => pos };
}

async function montar(persona: string | null): Promise<HTMLVideoElement> {
  await act(() => {
    render(
      <I18nProvider locale="es">
        <Player
          t={t}
          hls={null}
          webm="/media/v/t/video.webm"
          titulo="Prueba"
          reanudar={persona ? { persona, video: 'v1' } : null}
        />
      </I18nProvider>,
      host,
    );
  });
  return host.querySelector('video')!;
}

const reiniciar = (): HTMLButtonElement | undefined =>
  [...host.querySelectorAll('button')].find((b) => b.textContent === t('player.restart'));

test('al abrirlo sigue donde se dejó y ofrece empezar desde el principio', async () => {
  guardar('ana', { v1: { s: 125, t: 600, en: Date.now() } });
  const v = await montar('ana');
  const sim = simular(v, 600);
  await act(() => void v.dispatchEvent(new Event('loadedmetadata')));
  expect(sim.pos()).toBe(125);
  expect(host.querySelector('[role="status"]')!.textContent).toBe(
    t('player.resumedAt', { time: '2:05' }),
  );
  const boton = reiniciar();
  expect(boton).toBeDefined();
  await act(() => boton!.click());
  expect(sim.pos()).toBe(0);
  expect(reiniciar()).toBeUndefined();
  // Volver al principio olvida el punto: la próxima vez empieza de cero.
  expect(desde(leer('ana'), 'v1', Date.now())).toBeNull();
});

test('sin punto guardado, o sin sesión, empieza de cero y no ofrece nada', async () => {
  guardar('ana', { v1: { s: 125, t: 600, en: Date.now() } });
  const v = await montar(null);
  const sim = simular(v, 600);
  await act(() => void v.dispatchEvent(new Event('loadedmetadata')));
  expect(sim.pos()).toBe(0);
  expect(reiniciar()).toBeUndefined();
});

test('guarda mientras suena y al salir; al acabar lo olvida', async () => {
  const v = await montar('ana');
  const sim = simular(v, 600);
  await act(() => void v.dispatchEvent(new Event('loadedmetadata')));
  await v.play();
  v.currentTime = 42;
  await act(() => void v.dispatchEvent(new Event('timeupdate')));
  expect(desde(leer('ana'), 'v1', Date.now())).toBe(42);
  // Entre guardado y guardado no se escribe en cada `timeupdate`…
  v.currentTime = 44;
  await act(() => void v.dispatchEvent(new Event('timeupdate')));
  expect(desde(leer('ana'), 'v1', Date.now())).toBe(42);
  // …pero sí al salir de la página.
  window.dispatchEvent(new Event('pagehide'));
  expect(desde(leer('ana'), 'v1', Date.now())).toBe(44);
  await act(() => void v.dispatchEvent(new Event('ended')));
  expect(leer('ana')).toEqual({});
  expect(sim.pos()).toBe(44);
});

test('al desmontar guarda lo último que se vio', async () => {
  const v = await montar('ana');
  simular(v, 600);
  await act(() => void v.dispatchEvent(new Event('loadedmetadata')));
  v.currentTime = 300;
  await act(() => void v.dispatchEvent(new Event('timeupdate')));
  await act(() => render(null, host));
  expect(desde(leer('ana'), 'v1', Date.now())).toBe(300);
});

const base = {
  id: 'v1',
  title: 'Formación',
  owner: { sub: 'o', name: 'Olga' },
  status: 'ready',
  kind: 'upload',
  visibility: 'org',
  views: 3,
  duration_ms: 600_000,
  created_at: '2026-01-01T00:00:00Z',
  published_at: '2026-01-01T00:00:00Z',
  poster_url: null,
} as unknown as Parameters<typeof VideoCard>[0]['video'];

test('la tarjeta lleva la barra de lo visto y lo dice en su nombre', async () => {
  await act(() => render(<VideoCard t={t} locale="es" video={base} visto={0.4} />, host));
  const barra = host.querySelector<HTMLElement>('[data-visto]');
  expect(barra?.style.width).toBe('40%');
  expect(host.querySelector('a')!.getAttribute('aria-label')).toContain(
    t('card.watched', { percent: 40 }),
  );
});

test('sin nada visto, o en directo, la tarjeta no lleva barra', async () => {
  await act(() => render(<VideoCard t={t} locale="es" video={base} />, host));
  expect(host.querySelector('[data-visto]')).toBeNull();
  const directo = { ...base, status: 'live', kind: 'live' } as typeof base;
  await act(() => render(<VideoCard t={t} locale="es" video={directo} visto={0.4} />, host));
  expect(host.querySelector('[data-visto]')).toBeNull();
});
