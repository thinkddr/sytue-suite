/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useListReorder } from './useListReorder.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

test('reordenar distingue antes/después, rechaza destinos de otra lista y conserva el teclado', () => {
  const host = document.createElement('div');
  document.body.append(host);
  const cambios: number[][] = [];
  const original = document.elementsFromPoint;
  let destino: Element[] = [];
  document.elementsFromPoint = () => destino;
  function Fixture({ disabled = false }: { disabled?: boolean }) {
    const orden = useListReorder({
      ids: ['a', 'b', 'c'],
      container: () => host,
      disabled,
      onReorder: (desde, hasta) => {
        cambios.push([desde, hasta]);
      },
    });
    return (
      <ul>
        {['a', 'b', 'c'].map((id) => (
          <li key={id} {...orden.itemProps(id)}>
            <button {...orden.handleProps(id)}>{id}</button>
          </li>
        ))}
      </ul>
    );
  }
  const arrastrar = (el: Element, y: number): void => {
    el.dispatchEvent(
      new PointerEvent('pointerdown', { clientX: 50, clientY: 10, pointerId: 1, bubbles: true }),
    );
    el.dispatchEvent(
      new PointerEvent('pointermove', { clientX: 50, clientY: y, pointerId: 1, bubbles: true }),
    );
    el.dispatchEvent(
      new PointerEvent('pointerup', { clientX: 50, clientY: y, pointerId: 1, bubbles: true }),
    );
  };
  try {
    act(() => render(<Fixture />, host));
    const filas = host.querySelectorAll('li');
    filas[1]!.getBoundingClientRect = () => new DOMRect(0, 20, 100, 20);
    destino = [filas[1]!];
    arrastrar(filas[0]!.querySelector('button')!, 25); // a antes de b ya está en su sitio.
    expect(cambios).toEqual([]);
    arrastrar(filas[0]!.querySelector('button')!, 38);
    expect(cambios).toEqual([[0, 1]]);
    const ajena = document.createElement('li');
    ajena.dataset.reorderId = 'c';
    document.body.append(ajena);
    destino = [ajena];
    arrastrar(filas[0]!.querySelector('button')!, 40);
    expect(cambios).toHaveLength(1);
    ajena.remove();
    filas[2]!
      .querySelector('button')!
      .dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true }));
    expect(cambios).toEqual([
      [0, 1],
      [2, 1],
    ]);
    act(() => render(<Fixture disabled />, host));
    host
      .querySelector('button')!
      .dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true }));
    expect(cambios).toHaveLength(2);
  } finally {
    act(() => render(null, host));
    host.remove();
    document.elementsFromPoint = original;
  }
});
