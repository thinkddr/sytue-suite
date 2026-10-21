/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El selector de calidades: la recomendada marcada con su tamaño, lo que el
 * original no alcanza atenuado y explicado, nunca sin ninguna, y el aviso de
 * que no cabe en el plan.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import type { Estimacion } from '../lib/tipos.js';
import { QualityPicker } from './QualityPicker.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const e720: Estimacion = {
  options: [
    { quality: 1080, allowed: false, recommended: false, bytes: 0 },
    { quality: 720, allowed: true, recommended: true, bytes: 33_180_000 },
    { quality: 360, allowed: true, recommended: true, bytes: 11_430_000 },
  ],
  recommended: [720, 360],
  original_bytes: 100_000_000,
  duration_ms: 60_000,
  approximate: true,
};

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});
function montar(v: VNode): HTMLElement {
  host = document.createElement('div');
  document.body.appendChild(host);
  render(v, host);
  return host;
}
const t = traductor('es');
const casillas = (h: HTMLElement): HTMLInputElement[] =>
  Array.from(h.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'));

test('la recomendada marcada, con su tamaño, y 1080p explicado', () => {
  const h = montar(
    <QualityPicker
      t={t}
      locale="es"
      estimacion={e720}
      elegidas={[720, 360]}
      alCambiar={() => undefined}
    />,
  );
  const texto = h.textContent ?? '';
  expect(texto).toContain('Recomendada');
  expect(texto).toContain('720p · 360p');
  expect(h.querySelector<HTMLInputElement>('input[type="radio"]')!.checked).toBe(true);
  expect(texto).toContain('El original no llega a esta calidad.');
  expect(texto).toContain('Estimación aproximada');
  const [c1080, c720, c360] = casillas(h);
  expect(c1080!.disabled).toBe(true);
  expect(c1080!.checked).toBe(false);
  expect(c720!.checked && c360!.checked).toBe(true);
});

test('desmarcar avisa con la elección nueva; la última no se puede quitar', () => {
  const vistas: number[][] = [];
  let h = montar(
    <QualityPicker
      t={t}
      locale="es"
      estimacion={e720}
      elegidas={[720, 360]}
      alCambiar={(qs) => vistas.push(qs)}
    />,
  );
  casillas(h)[2]!.click();
  expect(vistas).toEqual([[720]]);
  render(null, host);
  host.remove();
  h = montar(
    <QualityPicker
      t={t}
      locale="es"
      estimacion={e720}
      elegidas={[720]}
      alCambiar={() => undefined}
    />,
  );
  expect(casillas(h)[1]!.disabled).toBe(true);
  // Fuera de la recomendada, su tarjeta no está marcada y elegirla vuelve a ella.
  const vuelta: number[][] = [];
  render(null, host);
  host.remove();
  h = montar(
    <QualityPicker
      t={t}
      locale="es"
      estimacion={e720}
      elegidas={[720]}
      alCambiar={(qs) => vuelta.push(qs)}
    />,
  );
  const radio = h.querySelector<HTMLInputElement>('input[type="radio"]')!;
  expect(radio.checked).toBe(false);
  radio.click();
  expect(vuelta).toEqual([[720, 360]]);
});

test('avisa si no cabe en lo que queda del plan', () => {
  const h = montar(
    <QualityPicker
      t={t}
      locale="es"
      estimacion={e720}
      elegidas={[720, 360]}
      alCambiar={() => undefined}
      disponibles={50_000_000}
    />,
  );
  expect(h.querySelector('[role="alert"]')?.textContent).toBe(
    'No cabe en el espacio que queda en tu plan.',
  );
});

test('mientras calcula, un esqueleto que lo dice', () => {
  const h = montar(
    <QualityPicker t={t} locale="es" estimacion={null} elegidas={[]} alCambiar={() => undefined} />,
  );
  expect(h.querySelector('[aria-busy="true"]')?.getAttribute('aria-label')).toBe(
    'Calculando cuánto ocupará…',
  );
});
