/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las paradas propias de un párrafo de Word (`--sytue-tabs`): cada tabulador recibe el hueco
 * hasta la suya, y lo que se guarda no cambia. happy-dom no maqueta: dónde cae cada tabulador
 * se le dice con getClientRects.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { leerParadas, quitarTabulaciones, tabular } from './tabulaciones';

if (typeof document === 'undefined') GlobalRegistrator.register();

const original = HTMLElement.prototype.getClientRects;
afterEach(() => {
  HTMLElement.prototype.getClientRects = original;
});

test('lee las paradas y salta lo que no entiende', () => {
  expect(leerParadas('120px left, 600.5px right dot, x')).toEqual([
    { pos: 120, alin: 'left', relleno: '' },
    { pos: 600.5, alin: 'right', relleno: 'dot' },
  ]);
});

test('cada tabulador llega a su parada y el HTML guardado no cambia', () => {
  const html =
    '<p style="white-space:pre-wrap;tab-size:48px;--sytue-tabs:120px left, 500px right dot">ab\tc\td</p><p>sin\tparadas</p>';
  const raiz = document.createElement('div');
  raiz.innerHTML = html;
  // El primer tabulador cae a 10 px del borde; el segundo, a 300.
  HTMLElement.prototype.getClientRects = function (this: HTMLElement) {
    const left = this.textContent === '\tc' ? 10 : 300;
    return [{ left, top: 0, width: 0, height: 0 }] as unknown as DOMRectList;
  };
  tabular(raiz);
  const vivos = Array.from(raiz.querySelectorAll<HTMLElement>('[data-tab-vivo]'));
  expect(vivos.map((s) => s.textContent)).toEqual(['\tc', '\td']);
  expect(vivos.map((s) => s.style.paddingLeft)).toEqual(['110px', '200px']);
  expect(vivos[0]?.style.background).toBe('');
  expect(vivos[1]?.style.background).toContain('radial-gradient');
  expect(quitarTabulaciones(raiz.innerHTML)).toBe(html);
  // Volver a pasar no anida envoltorios.
  tabular(raiz);
  expect(raiz.querySelectorAll('[data-tab-vivo]')).toHaveLength(2);
});
