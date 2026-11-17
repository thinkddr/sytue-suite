/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tesela de la papelera dice cuándo se borra la foto, y no promete nada cuando la
 * papelera no caduca.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import { PlazoDePurga } from './Trash.js';

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
function texto(dias: number | null, locale: 'es' | 'en' = 'es'): string {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(<PlazoDePurga t={traductor(locale)} dias={dias} />, host));
  return host.textContent ?? '';
}

test('dice en cuántos días se borra, con su plural', () => {
  expect(texto(12)).toBe('Se borrará en 12 días');
  expect(texto(1)).toBe('Se borrará en 1 día');
  expect(texto(3, 'en')).toBe('Deleted in 3 days');
});

test('el último día dice «hoy», no «en 0 días»', () => {
  expect(texto(0)).toBe('Se borrará hoy');
});

test('sin fecha de purga no pinta nada', () => {
  expect(texto(null)).toBe('');
});
