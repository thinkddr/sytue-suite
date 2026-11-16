/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL TABLERO SE USA SIN IR A LA REJILLA: cada columna añade su tarjeta (con el valor de
 * esa columna), la tarjeta se pulsa para abrir su ficha y «Mover a» vive en el menú «⋯»
 * de la tarjeta, que sigue siendo la alternativa de teclado al arrastre (WCAG 2.5.7).
 * Quien solo lee no ve ni «Añadir» ni «Mover a».
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import { Tablero } from './Tablero.js';
import { SIN_VALOR } from '../lib/vistas.js';

// Solo aquí, y se retira al acabar: el resto de la suite corre sin DOM.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});
afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
});

const t = ((k: string, v?: Record<string, unknown>) =>
  v ? `${k}:${Object.values(v).join(',')}` : k) as never;

const NOMBRE: BasesField = { id: 'nom', table_id: 't1', name: 'Nombre', kind: 'text', position: 0 };
const ESTADO: BasesField = {
  id: 'est',
  table_id: 't1',
  name: 'Estado',
  kind: 'select',
  choices: ['Por hacer', 'En curso', 'Hecho'],
  position: 1,
};
const reg = (id: string, data: BasesRecord['data']): BasesRecord => ({
  id,
  table_id: 't1',
  data,
  links: {},
  rev: 1,
  created_by: '',
  updated_by: '',
  created_at: '',
  updated_at: '',
});
const REGISTROS = [reg('r1', { nom: 'Ana', est: 'Por hacer' }), reg('r2', { nom: 'Bea' })];

function pintar(puedeEditar = true) {
  const anadidos: string[] = [];
  const abiertos: string[] = [];
  const movidos: [string, BasesValue][] = [];
  act(() => {
    render(
      <Tablero
        t={t}
        campos={[NOMBRE, ESTADO]}
        agrupar={ESTADO}
        registros={REGISTROS}
        puedeEditar={puedeEditar}
        onMover={async (r, v) => {
          movidos.push([r.id, v]);
          return null;
        }}
        onAbrir={(r) => abiertos.push(r.id)}
        onAnadir={(v) => anadidos.push(v)}
      />,
      document.body,
    );
  });
  return { anadidos, abiertos, movidos };
}

const porEtiqueta = (etiqueta: string): HTMLElement | null =>
  document.querySelector<HTMLElement>(`[aria-label="${etiqueta}"]`);

test('«Añadir tarjeta» de una columna pide crearla con el valor de esa columna', () => {
  const { anadidos } = pintar();
  act(() => porEtiqueta('board.addIn:En curso')!.click());
  act(() => porEtiqueta('board.addIn:board.noValue')!.click());
  expect(anadidos).toEqual(['En curso', SIN_VALOR]);
});

test('la tarjeta se pulsa para abrir su ficha, por su nombre o por cualquier parte', () => {
  const { abiertos } = pintar();
  act(() => porEtiqueta('board.open:Ana')!.click());
  act(() => document.querySelector<HTMLElement>('[data-bases-card="r2"]')!.click());
  expect(abiertos).toEqual(['r1', 'r2']);
});

test('«Mover a» va en el menú «⋯» de la tarjeta, sin selector siempre a la vista', async () => {
  const { movidos, abiertos } = pintar();
  expect(document.querySelector('select')).toBeNull();
  act(() => porEtiqueta('board.actions:Ana')!.click());
  const items = [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')];
  const textos = items.map((i) => i.textContent);
  // Las otras columnas, no la suya.
  expect(textos).toContain('board.moveToColumn:En curso');
  expect(textos).toContain('board.moveToColumn:board.noValue');
  expect(textos).not.toContain('board.moveToColumn:Por hacer');
  await act(async () => items.find((i) => i.textContent === 'board.moveToColumn:Hecho')!.click());
  expect(movidos).toEqual([['r1', 'Hecho']]);
  // Abrir el menú no abre la ficha.
  expect(abiertos).toEqual([]);
});

test('una columna vacía invita a soltar una tarjeta', () => {
  pintar();
  const columna = document.querySelector('[data-bases-column="Hecho"]')!;
  expect(columna.textContent).toContain('board.empty');
});

test('quien solo lee no añade ni mueve, pero abre la ficha', () => {
  const { abiertos } = pintar(false);
  expect(porEtiqueta('board.addIn:En curso')).toBeNull();
  expect(document.body.textContent).not.toContain('board.empty');
  act(() => porEtiqueta('board.actions:Ana')!.click());
  const textos = [...document.querySelectorAll('[role="menuitem"]')].map((i) => i.textContent);
  expect(textos.some((x) => x?.startsWith('board.moveToColumn'))).toBe(false);
  act(() => porEtiqueta('board.open:Ana')!.click());
  expect(abiertos).toEqual(['r1']);
});

test('pulsar el asa sin arrastrar (o con Enter) abre el menú con «Mover a»', () => {
  pintar();
  expect(document.querySelector('[role="menuitem"]')).toBeNull();
  act(() => porEtiqueta('board.moveTo:Ana')!.click());
  const textos = [...document.querySelectorAll('[role="menuitem"]')].map((i) => i.textContent);
  expect(textos).toContain('board.moveToColumn:Hecho');
});
