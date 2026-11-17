/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La barra de años: un botón por año con fotos, el elegido marcado, y nada
 * cuando no hay adónde saltar.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import { BarraDeAnyos } from './BarraDeAnyos.js';

let registrado = false;
beforeAll(() => {
  if (typeof document !== 'undefined') return;
  GlobalRegistrator.register();
  registrado = true;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});

const ANYOS = [
  { year: 2024, count: 120 },
  { year: 2021, count: 3 },
  { year: 2019, count: 40 },
];

function montar(activo: number | null, onElegir: (a: number | null) => void = () => {}) {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() =>
    render(
      <BarraDeAnyos t={traductor('es')} anyos={ANYOS} activo={activo} onElegir={onElegir} />,
      host,
    ),
  );
  return [...host.querySelectorAll('button')];
}

test('un botón por año, con «Recientes» delante, dentro de una navegación con nombre', () => {
  const botones = montar(null);
  expect(botones.map((b) => b.textContent)).toEqual(['Recientes', '2024', '2021', '2019']);
  expect(host.querySelector('nav')?.getAttribute('aria-label')).toBe('Saltar a un año');
  expect(botones[3]?.getAttribute('title')).toBe('40 fotos');
});

test('marca el año elegido y avisa del que se pulsa', () => {
  const elegidos: (number | null)[] = [];
  const botones = montar(2021, (a) => elegidos.push(a));
  expect(botones.map((b) => b.getAttribute('aria-pressed'))).toEqual([
    'false',
    'false',
    'true',
    'false',
  ]);
  act(() => botones[3]?.click());
  act(() => botones[0]?.click());
  expect(elegidos).toEqual([2019, null]);
});

test('con un solo año no hay barra', () => {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() =>
    render(
      <BarraDeAnyos
        t={traductor('es')}
        anyos={[{ year: 2024, count: 5 }]}
        activo={null}
        onElegir={() => {}}
      />,
      host,
    ),
  );
  expect(host.innerHTML).toBe('');
});
