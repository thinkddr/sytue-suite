/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Checks de `usePointerDrag`. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { usePointerDrag } from './usePointerDrag';

if (typeof document === 'undefined') GlobalRegistrator.register();

describe('usePointerDrag', () => {
  test('inicia arrastre y notifica onMove y onEnd', () => {
    const host = document.createElement('div');
    document.body.appendChild(host);

    const moves: { dxRaw: number; dyRaw: number; data: string }[] = [];
    let finData: string | null = null;

    function TestComp() {
      const { iniciarArrastre, onPointerMove, onPointerUp } = usePointerDrag<string>({
        escala: 1,
        onMove: (ev) => moves.push({ dxRaw: ev.dxRaw, dyRaw: ev.dyRaw, data: ev.data }),
        onEnd: (data) => {
          finData = data;
        },
      });

      return (
        <div
          id="arrastrable"
          onPointerDown={(e) => iniciarArrastre(e, 'caja-1')}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
        />
      );
    }

    act(() => {
      render(<TestComp />, host);
    });

    const el = host.querySelector('#arrastrable') as HTMLElement;
    expect(el).not.toBeNull();

    // Simular pointerdown
    const evDown = new PointerEvent('pointerdown', {
      clientX: 50,
      clientY: 50,
      pointerId: 1,
      bubbles: true,
      cancelable: true,
    });
    el.dispatchEvent(evDown);

    // Simular pointermove
    const evMove1 = new PointerEvent('pointermove', {
      clientX: 65,
      clientY: 80,
      pointerId: 1,
      bubbles: true,
      cancelable: true,
    });
    el.dispatchEvent(evMove1);

    expect(moves.length).toBe(1);
    expect(moves[0]).toEqual({ dxRaw: 15, dyRaw: 30, data: 'caja-1' });

    // Simular pointerup
    const evUp = new PointerEvent('pointerup', {
      clientX: 65,
      clientY: 80,
      pointerId: 1,
      bubbles: true,
      cancelable: true,
    });
    el.dispatchEvent(evUp);

    expect(finData).toBe('caja-1');

    act(() => {
      render(null, host);
    });
    host.remove();
  });

  test('normaliza deltas cuando se especifica escenario y escala', () => {
    const host = document.createElement('div');
    document.body.appendChild(host);

    const deltas: { dxRaw: number; dyRaw: number }[] = [];

    function TestComp() {
      const { iniciarArrastre, onPointerMove, onPointerUp } = usePointerDrag<string>({
        escala: 2,
        escenario: { w: 500, h: 500 },
        onMove: (ev) => deltas.push({ dxRaw: ev.dxRaw, dyRaw: ev.dyRaw }),
      });

      return (
        <div
          id="target"
          onPointerDown={(e) => iniciarArrastre(e, 'caja')}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
        />
      );
    }

    act(() => {
      render(<TestComp />, host);
    });

    const el = host.querySelector('#target') as HTMLElement;

    el.dispatchEvent(
      new PointerEvent('pointerdown', {
        clientX: 100,
        clientY: 100,
        pointerId: 2,
        bubbles: true,
        cancelable: true,
      }),
    );

    // dx = 100px. Con escenario 500 y escala 2, total px virtuales = 1000px.
    // (100 / 1000) * 100 = 10%
    el.dispatchEvent(
      new PointerEvent('pointermove', {
        clientX: 200,
        clientY: 250,
        pointerId: 2,
        bubbles: true,
        cancelable: true,
      }),
    );

    expect(deltas.length).toBe(1);
    expect(deltas[0]?.dxRaw).toBe(10);
    expect(deltas[0]?.dyRaw).toBe(15);

    act(() => {
      render(null, host);
    });
    host.remove();
  });
});

test('cancelar distingue restaurar del commit y solo termina una vez', () => {
  const host = document.createElement('div');
  document.body.append(host);
  const commits: string[] = [],
    cancels: string[] = [];
  function Fixture({ restore }: { restore: boolean }) {
    const drag = usePointerDrag<string>({
      onEnd: (data) => commits.push(data),
      ...(restore ? { onCancel: (data: string) => cancels.push(data) } : {}),
    });
    return (
      <div
        onPointerDown={(e) => drag.iniciarArrastre(e, 'nota')}
        onPointerCancel={drag.onPointerCancel}
        onPointerUp={drag.onPointerUp}
      />
    );
  }
  for (const restore of [false, true]) {
    act(() => render(<Fixture restore={restore} />, host));
    const el = host.firstElementChild!;
    for (const type of ['pointerdown', 'pointercancel', 'pointerup']) {
      el.dispatchEvent(new PointerEvent(type, { pointerId: 1, bubbles: true }));
    }
  }
  expect(commits).toEqual(['nota']);
  expect(cancels).toEqual(['nota']);
  act(() => render(null, host));
  host.remove();
});
