/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Presentar con Cast» desde el lado de quien mira: la diapositiva en la que va el directo
 * y los capítulos de la grabación (sobre un cliente de mentira).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, mock, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { Presentacion } from '../lib/tipos.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

let respuesta: Presentacion | null = null;
void mock.module('../client/castApi.js', () => ({
  presentacion: () => Promise.resolve(respuesta),
}));

const { Diapositivas } = await import('./Diapositivas.js');
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

const presentacion = (extra: Partial<Presentacion> = {}): Presentacion => ({
  index: 2,
  total: 12,
  title: 'Cuentas del pozo',
  started_at: '2026-10-03T10:00:00Z',
  updated_at: '2026-10-03T10:05:00Z',
  marks: [
    { at_ms: 0, index: 0, title: 'Portada' },
    { at_ms: 65_000, index: 1, title: '' },
    { at_ms: 3_725_000, index: 2, title: 'Cuentas del pozo' },
  ],
  ...extra,
});

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});
async function pintar(
  enDirecto: boolean,
  saltar: (ms: number) => void = () => undefined,
): Promise<void> {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <Diapositivas t={t} videoId="v1" conSesion enDirecto={enDirecto} activo saltar={saltar} />,
      host,
    );
  });
  await act(async () => {
    await Promise.resolve();
  });
}

test('en directo dice en qué diapositiva va y su título', async () => {
  respuesta = presentacion();
  await pintar(true);
  expect(host.textContent).toContain('Diapositiva 3 de 12');
  expect(host.textContent).toContain('Cuentas del pozo');
  expect(host.querySelector('[aria-live="polite"]')).not.toBeNull();
  expect(host.querySelectorAll('button').length).toBe(0);
});

test('la grabación lista un capítulo por diapositiva y salta a su instante', async () => {
  respuesta = presentacion();
  const saltos: number[] = [];
  await pintar(false, (ms) => saltos.push(ms));
  const botones = [...host.querySelectorAll('button')];
  expect(botones.length).toBe(3);
  expect(botones[1]?.textContent).toContain('1:05');
  expect(botones[2]?.textContent).toContain('1:02:05');
  expect(botones[2]?.textContent).toContain('Cuentas del pozo');
  await act(async () => {
    botones[1]?.click();
  });
  expect(saltos).toEqual([65_000]);
});

test('sin presentación, o sin capítulos en la grabación, no pinta nada', async () => {
  respuesta = null;
  await pintar(true);
  expect(host.textContent).toBe('');
  render(null, host);
  host.remove();
  respuesta = presentacion({ marks: [] });
  await pintar(false);
  expect(host.textContent).toBe('');
});
