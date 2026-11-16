/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «IR A UNA FECHA» (tecla g): el campo arranca en el día que se ve y «Ir» lleva al
 * día elegido, a medianoche local. Sin una fecha válida el botón no hace nada.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { IrAFecha } from './IrAFecha.js';

// Solo aquí, y se retira al acabar: el resto de la suite corre sin DOM.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;
let raiz: HTMLElement | null = null;
afterEach(() => {
  if (raiz) render(null, raiz);
  raiz?.remove();
  raiz = null;
});

function montar(cursor: Date, idas: Date[], cierres: { n: number }): void {
  raiz = document.createElement('div');
  document.body.appendChild(raiz);
  act(() => {
    render(
      <IrAFecha
        t={t}
        open
        cursor={cursor}
        onIr={(d) => idas.push(d)}
        onClose={() => cierres.n++}
      />,
      raiz!,
    );
  });
}

const campo = (): HTMLInputElement =>
  document.querySelector<HTMLInputElement>('#ir-a-fecha') as HTMLInputElement;
const botonIr = (): HTMLButtonElement =>
  document.querySelector<HTMLButtonElement>('button[form="form-ir-a-fecha"]') as HTMLButtonElement;

test('arranca en el día que se ve y «Ir» lleva al elegido', () => {
  const idas: Date[] = [];
  const cierres = { n: 0 };
  montar(new Date(2026, 10, 11, 15, 30), idas, cierres);
  expect(campo().type).toBe('date');
  expect(campo().value).toBe('2026-11-11');

  act(() => {
    campo().value = '2031-07-04';
    campo().dispatchEvent(new Event('input', { bubbles: true }));
  });
  act(() => {
    document
      .querySelector('#form-ir-a-fecha')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  });
  expect(idas).toHaveLength(1);
  expect([idas[0]!.getFullYear(), idas[0]!.getMonth(), idas[0]!.getDate()]).toEqual([2031, 6, 4]);
  expect(cierres.n).toBe(1);
});

test('con el campo vacío no se mueve ni se cierra', () => {
  const idas: Date[] = [];
  const cierres = { n: 0 };
  montar(new Date(2026, 10, 11), idas, cierres);
  act(() => {
    campo().value = '';
    campo().dispatchEvent(new Event('input', { bubbles: true }));
  });
  expect(botonIr().disabled).toBe(true);
  act(() => {
    document
      .querySelector('#form-ir-a-fecha')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  });
  expect(idas).toHaveLength(0);
  expect(cierres.n).toBe(0);
});
