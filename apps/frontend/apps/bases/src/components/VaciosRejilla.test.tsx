/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS VACÍOS DE LA REJILLA LLEVAN SU PASO SIGUIENTE: sin campos, «Añadir campo» para el
 * propietario; sin registros, «Añadir registro» para quien edita (y no repetido al
 * pie); con un recorte que no devuelve nada, «Ver todos los registros». Quien solo lee
 * ve el porqué, sin botones.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { BasesField } from '@sytue/api-clients/bases';
import { Rejilla, type RejillaProps } from './Rejilla.js';

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

const CAMPOS: BasesField[] = [
  { id: 'nom', table_id: 't1', name: 'Nombre', kind: 'text', position: 0 },
];

function montar(extra: Partial<RejillaProps>): string[] {
  const hechos: string[] = [];
  act(() => {
    render(
      <Rejilla
        t={t}
        nombreTabla="Pedidos"
        campos={CAMPOS}
        registros={[]}
        puedeEditar={false}
        etiquetas={{}}
        onCambiar={async () => null}
        onElegirVarios={() => undefined}
        onAdjuntos={() => undefined}
        onAbrir={() => undefined}
        onNuevo={() => hechos.push('nuevo')}
        onVerTodos={() => hechos.push('todos')}
        desde={0}
        hayAntes={false}
        onAntes={() => undefined}
        hayMas={false}
        onMas={() => undefined}
        {...extra}
      />,
      document.body,
    );
  });
  return hechos;
}

const titulo = () => document.querySelector('h3')?.textContent;
const acciones = () =>
  [...document.querySelectorAll<HTMLElement>('[data-accion-vacio]')].map(
    (b) => b.dataset.accionVacio,
  );
const textos = () => [...document.querySelectorAll('button')].map((b) => b.textContent?.trim());

test('sin campos: el propietario puede añadir el primero; quien no lo es, no', () => {
  const nuevos: number[] = [];
  montar({
    campos: [],
    puedeEditar: true,
    onCampo: () => undefined,
    onNuevoCampo: () => nuevos.push(1),
  });
  expect(titulo()).toBe('grid.noFields');
  expect(document.body.textContent).toContain('grid.noFields.owner');
  expect(acciones()).toEqual(['anadirCampo']);
  act(() => document.querySelector<HTMLButtonElement>('[data-accion-vacio]')!.click());
  expect(nuevos).toEqual([1]);

  montar({ campos: [], puedeEditar: true });
  expect(titulo()).toBe('grid.noFields');
  expect(document.body.textContent).toContain('grid.noFields.reader');
  expect(acciones()).toEqual([]);
});

test('sin registros: quien edita añade uno desde el vacío, sin repetirlo al pie', () => {
  const hechos = montar({ puedeEditar: true });
  expect(titulo()).toBe('grid.empty');
  expect(acciones()).toEqual(['anadirRegistro']);
  expect(textos().filter((x) => x === 'grid.addRecord')).toHaveLength(1);
  act(() => document.querySelector<HTMLButtonElement>('[data-accion-vacio]')!.click());
  expect(hechos).toEqual(['nuevo']);
});

test('sin registros y solo lectura: el porqué, sin botones', () => {
  montar({ puedeEditar: false });
  expect(titulo()).toBe('grid.empty');
  expect(document.body.textContent).toContain('grid.empty.reader');
  expect(acciones()).toEqual([]);
  expect(textos()).not.toContain('grid.addRecord');
});

test('un recorte vacío ofrece ver todos los registros, también a quien solo lee', () => {
  let hechos = montar({ recorte: 'vista' });
  expect(titulo()).toBe('grid.viewEmpty');
  expect(acciones()).toEqual(['verTodos']);
  act(() => document.querySelector<HTMLButtonElement>('[data-accion-vacio]')!.click());
  expect(hechos).toEqual(['todos']);

  hechos = montar({ recorte: 'busqueda', puedeEditar: true });
  expect(titulo()).toBe('grid.noMatches');
  expect(acciones()).toEqual(['verTodos']);
  // Añadir sigue al pie: el recorte no es la tabla vacía.
  expect(textos()).toContain('grid.addRecord');
});
