// Checks del restablecimiento de escala. Ejecutar: `bun test`.
//
// LO QUE SE VIGILA NO ES QUE EL ZOOM VUELVA —eso solo se ve en un iPhone— sino
// que el `maximum-scale=1` SE RETIRE. Si se quedara puesto, el arreglo del zoom
// costaría el pellizco para ampliar, que es como mucha gente lee una pantalla
// pequeña. Es una restricción de accesibilidad y no una preferencia.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (!globalThis.document) GlobalRegistrator.register();

import { expect, test } from 'bun:test';
import { restaurarZoomDelViewport } from './zoom.js';

const VIEWPORT = 'width=device-width, initial-scale=1, interactive-widget=resizes-content';

function conViewport(content: string): HTMLMetaElement {
  document.head.innerHTML = '';
  const m = document.createElement('meta');
  m.name = 'viewport';
  m.content = content;
  document.head.append(m);
  return m;
}

/** Deja pasar los dos fotogramas que la función espera antes de retirarlo. */
const dosFotogramas = (): Promise<void> =>
  new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(() => r())));

test('pone maximum-scale=1 para forzar el recálculo', () => {
  const m = conViewport(VIEWPORT);
  restaurarZoomDelViewport();
  expect(m.content).toContain('maximum-scale=1');
});

test('Y LO RETIRA: dejarlo puesto quita el pellizco para ampliar', async () => {
  const m = conViewport(VIEWPORT);
  restaurarZoomDelViewport();
  await dosFotogramas();
  expect(m.content).toBe(VIEWPORT);
  expect(m.content).not.toContain('maximum-scale');
});

test('no toca un viewport que YA fija maximum-scale: esa es decisión de esa página', () => {
  const fijado = `${VIEWPORT}, maximum-scale=2`;
  const m = conViewport(fijado);
  restaurarZoomDelViewport();
  expect(m.content).toBe(fijado);
});

test('suelta el foco: sin eso Safari lo mantiene ampliado sobre el campo', () => {
  conViewport(VIEWPORT);
  const input = document.createElement('input');
  document.body.append(input);
  input.focus();
  expect(document.activeElement).toBe(input);
  restaurarZoomDelViewport();
  expect(document.activeElement).not.toBe(input);
});

test('sin meta viewport no revienta', () => {
  document.head.innerHTML = '';
  expect(() => restaurarZoomDelViewport()).not.toThrow();
});
