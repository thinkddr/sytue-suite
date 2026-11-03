/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useLongPress } from './useLongPress.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const esperar = () => new Promise((r) => setTimeout(r, 480));
const puntero = (tipo: string, extra: PointerEventInit = {}) =>
  new PointerEvent(tipo, {
    pointerId: 1,
    pointerType: 'touch',
    isPrimary: true,
    clientX: 10,
    clientY: 10,
    bubbles: true,
    cancelable: true,
    ...extra,
  });

test('mantener selecciona el origen, no abre y no contamina el siguiente clic de ratón', async () => {
  const host = document.createElement('div');
  document.body.append(host);
  const elegidos: string[] = [];
  let abiertos = 0;
  function Fixture({ id }: { id: string }) {
    const hold = useLongPress<string>((data) => elegidos.push(data));
    return (
      <button
        onPointerDown={(e) => hold.iniciar(e, id)}
        onClickCapture={hold.onClickCapture}
        onContextMenuCapture={hold.onContextMenuCapture}
        onClick={() => {
          abiertos++;
        }}
      >
        Contacto
      </button>
    );
  }
  try {
    act(() => render(<Fixture id="origen" />, host));
    const el = host.firstElementChild!;
    el.dispatchEvent(puntero('pointerdown'));
    act(() => render(<Fixture id="reciclado" />, host));
    await esperar();
    expect(elegidos).toEqual(['origen']);
    el.dispatchEvent(puntero('pointerup'));
    el.dispatchEvent(new MouseEvent('click', { detail: 1, bubbles: true, cancelable: true }));
    expect(abiertos).toBe(0);
    el.dispatchEvent(puntero('pointerdown'));
    await esperar();
    el.dispatchEvent(puntero('pointercancel'));
    el.dispatchEvent(puntero('pointerdown', { pointerType: 'mouse' }));
    el.dispatchEvent(new MouseEvent('click', { detail: 1, bubbles: true }));
    expect(abiertos).toBe(1);
  } finally {
    act(() => render(null, host));
    host.remove();
  }
});

test('desplazar, cancelar o apoyar un segundo dedo no selecciona', async () => {
  const host = document.createElement('div');
  document.body.append(host);
  let elegidos = 0;
  function Fixture() {
    const hold = useLongPress<undefined>(() => {
      elegidos++;
    });
    return <button onPointerDown={(e) => hold.iniciar(e, undefined)}>Fila</button>;
  }
  try {
    act(() => render(<Fixture />, host));
    const el = host.firstElementChild!;
    el.dispatchEvent(puntero('pointerdown'));
    document.dispatchEvent(puntero('pointermove', { clientY: 30 }));
    el.dispatchEvent(puntero('pointerdown'));
    document.dispatchEvent(puntero('pointercancel'));
    el.dispatchEvent(puntero('pointerdown', { isPrimary: false }));
    el.dispatchEvent(puntero('pointerdown'));
    // El segundo dedo puede caer en otra fila y no pasar por este hook.
    document.body.dispatchEvent(puntero('pointerdown', { pointerId: 2, isPrimary: false }));
    await esperar();
    expect(elegidos).toBe(0);
  } finally {
    act(() => render(null, host));
    host.remove();
  }
});
