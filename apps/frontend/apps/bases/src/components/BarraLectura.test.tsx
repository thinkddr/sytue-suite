/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BUSCAR Y ORDENAR SIN GUARDAR UNA VISTA: la barra traduce lo que se teclea y elige a
 * los parámetros que resuelve el servicio (`q`, `sort`), espera a que se deje de
 * teclear y no ofrece ordenar por enlaces ni adjuntos.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { BasesField } from '@sytue/api-clients/bases';
import { BarraLectura, ESPERA_BUSQUEDA_MS } from './BarraLectura.js';
import { SIN_LECTURA, opcionesDeLectura, type Lectura } from '../lib/lectura.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});
afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
});

const t = ((k: string) => k) as never;
const campo = (id: string, kind: BasesField['kind']): BasesField => ({
  id,
  table_id: 't1',
  name: id.toUpperCase(),
  kind,
  position: 0,
});
const campos = [campo('nombre', 'text'), campo('cifra', 'number'), campo('enlace', 'link')];

test('opcionesDeLectura solo envía lo pedido', () => {
  expect(opcionesDeLectura(SIN_LECTURA)).toEqual({});
  expect(opcionesDeLectura({ q: '  ana ', campo: 'cifra', dir: 'desc' })).toEqual({
    q: 'ana',
    sort: 'cifra:desc',
  });
});

test('la búsqueda espera a que se deje de teclear y el orden se aplica al elegirlo', async () => {
  const cambios: Lectura[] = [];
  let lectura = SIN_LECTURA;
  const pintar = () =>
    render(
      <BarraLectura
        t={t}
        campos={campos}
        lectura={lectura}
        onCambiar={(l) => {
          cambios.push(l);
          lectura = l;
          pintar();
        }}
      />,
      document.body,
    );
  act(pintar);
  const opciones = [...document.querySelectorAll('option')].map((o) => o.value);
  expect(opciones).toEqual(['', 'nombre', 'cifra']);

  const input = document.querySelector('input[type="search"]') as HTMLInputElement;
  act(() => {
    input.value = 'a';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.value = 'ana';
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
  expect(cambios).toHaveLength(0);
  await act(() => new Promise((r) => setTimeout(r, ESPERA_BUSQUEDA_MS + 50)));
  expect(cambios).toEqual([{ q: 'ana', campo: '', dir: 'asc' }]);

  const select = document.querySelector('select') as HTMLSelectElement;
  act(() => {
    select.value = 'cifra';
    select.dispatchEvent(new Event('change', { bubbles: true }));
  });
  expect(cambios.at(-1)).toEqual({ q: 'ana', campo: 'cifra', dir: 'asc' });
  const dir = document.querySelector('button[aria-label="grid.sortAsc"]') as HTMLButtonElement;
  act(() => dir.click());
  expect(cambios.at(-1)).toEqual({ q: 'ana', campo: 'cifra', dir: 'desc' });
});
