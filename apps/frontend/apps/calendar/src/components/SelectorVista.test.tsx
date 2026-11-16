/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL SELECTOR DE VISTA ES UN GRUPO DE RADIOS.
 *
 * Eran cuatro botones con `aria-pressed`: cuatro paradas de Tab y ninguna flecha. Ahora
 * es el `SegmentedControl` de `@sytue/ui`: una parada, flechas que cambian de vista y,
 * en el cajón del teléfono, la rejilla de 2×2 que trunca antes que desbordar.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { ViewMode } from './dates.js';
import { SelectorVista } from './SelectorVista.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterEach(() => render(null, document.body));
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string, v?: Record<string, string>) =>
  v ? `${v['label']} (${v['key']})` : k) as never;

function montar(rejilla = false): { host: HTMLElement; vistas: ViewMode[] } {
  const vistas: ViewMode[] = [];
  const host = document.createElement('div');
  document.body.appendChild(host);
  const pintar = (m: ViewMode) =>
    render(
      <SelectorVista
        t={t}
        mode={m}
        rejilla={rejilla}
        onChange={(n) => {
          vistas.push(n);
          pintar(n);
        }}
      />,
      host,
    );
  act(() => pintar('week'));
  return { host, vistas };
}

const radios = (host: HTMLElement) =>
  Array.from(host.querySelectorAll<HTMLButtonElement>('[role="radio"]'));

test('un radiogroup con nombre, la vista marcada y su atajo en el title', () => {
  const { host } = montar();
  const grupo = host.querySelector('[role="radiogroup"]')!;
  expect(grupo.getAttribute('aria-label')).toBe('view.label');
  expect(host.querySelector('[aria-pressed]')).toBeNull();
  expect(radios(host).map((r) => r.getAttribute('aria-checked'))).toEqual([
    'false',
    'true',
    'false',
    'false',
  ]);
  expect(radios(host).map((r) => r.title)).toEqual([
    'view.month (M)',
    'view.week (W)',
    'view.day (D)',
    'view.agenda (A)',
  ]);
  expect(radios(host).map((r) => r.tabIndex)).toEqual([-1, 0, -1, -1]);
});

test('las flechas cambian de vista; en el cajón ↓ baja de fila', () => {
  const fila = montar();
  act(() => {
    radios(fila.host)[1]!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }),
    );
  });
  expect(fila.vistas).toEqual(['day']);
  render(null, document.body);

  const cajon = montar(true);
  expect(cajon.host.querySelector('[role="radiogroup"]')!.className).toContain('w-full');
  act(() => {
    radios(cajon.host)[1]!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }),
    );
  });
  expect(cajon.vistas).toEqual(['agenda']);
});
