/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA FICHA DEL REGISTRO: cada campo con su control, el foco en el primero, un valor que
 * no encaja se marca sin enviar nada, «Guardar» manda solo lo cambiado y quien solo lee
 * no tiene nada que escriba. Es lo que prueban `TestLaFichaRellenaElRegistro` y
 * `TestQuienSoloLeeNoEdita` en la ficha de Fyne.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import { FichaRegistro, type FichaRegistroProps } from './FichaRegistro.js';

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

const campo = (id: string, name: string, kind: BasesField['kind'], extra = {}): BasesField => ({
  id,
  table_id: 't1',
  name,
  kind,
  position: 0,
  ...extra,
});

const CAMPOS = [
  campo('nom', 'Nombre', 'text'),
  campo('eda', 'Edad', 'number'),
  campo('act', 'Activa', 'checkbox'),
  campo('niv', 'Nivel', 'select', { choices: ['Plata', 'Oro'] }),
  campo('emp', 'Empresa', 'link', { link_table_id: 't2' }),
  campo('tot', 'Total', 'formula', { formula: '{eda}', result: 'number' }),
];

const REGISTRO: BasesRecord = {
  id: 'r1',
  table_id: 't1',
  data: { nom: 'Ana', eda: 21, tot: 21 },
  links: {},
  rev: 1,
  created_by: '',
  updated_by: '',
  created_at: '',
  updated_at: '',
};

function montar(extra: Partial<FichaRegistroProps> = {}) {
  const guardados: [Record<string, BasesValue>, Record<string, string[]>, boolean][] = [];
  const borrados: number[] = [];
  act(() => {
    render(
      <FichaRegistro
        t={t}
        campos={CAMPOS}
        registro={REGISTRO}
        actual={REGISTRO}
        n={2}
        nuevo={false}
        puedeEditar
        opciones={{ t2: [{ id: 'e1', etiqueta: 'Acme' }] }}
        etiquetas={{ t2: new Map([['e1', 'Acme']]) }}
        onGuardar={(d, e, o) => guardados.push([d, e, o])}
        onBorrar={() => borrados.push(1)}
        onAdjuntos={() => undefined}
        onCerrar={() => undefined}
        {...extra}
      />,
      document.body,
    );
  });
  return { guardados, borrados };
}

const campoDe = (nombre: string) =>
  [...document.querySelectorAll<HTMLLabelElement>('label')].find(
    (l) => l.textContent?.trim() === nombre,
  )?.control as HTMLInputElement | HTMLSelectElement | undefined;

const boton = (texto: string) =>
  [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === texto,
  );

function escribir(el: HTMLInputElement, valor: string): void {
  act(() => {
    el.value = valor;
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

test('la ficha pide cada campo con su control y el foco va al primero', () => {
  montar();
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('grid.row:2');
  expect(campoDe('Nombre')?.value).toBe('Ana');
  expect(document.activeElement).toBe(campoDe('Nombre')!);
  expect(campoDe('Nivel')?.tagName).toBe('SELECT');
  expect(campoDe('Activa')?.type).toBe('checkbox');
  expect(campoDe('Acme')?.type).toBe('checkbox');
  // La fórmula se lee: no hay control que la escriba.
  expect(campoDe('Total')).toBeUndefined();
  expect(document.body.textContent).toContain('grid.formulaReadOnly');
});

test('un valor que no encaja se marca, no se envía y no borra lo demás', () => {
  const { guardados } = montar();
  escribir(campoDe('Nombre') as HTMLInputElement, 'Eva');
  escribir(campoDe('Edad') as HTMLInputElement, 'treinta');
  act(() => boton('common.save')!.click());
  expect(guardados).toEqual([]);
  expect(campoDe('Edad')?.getAttribute('aria-invalid')).toBe('true');
  expect(document.activeElement).toBe(campoDe('Edad')!);
  expect(document.body.textContent).toContain('grid.invalidValue');
  expect(campoDe('Nombre')?.value).toBe('Eva');
  // Corregido, viaja solo lo cambiado, ya convertido a su tipo.
  escribir(campoDe('Edad') as HTMLInputElement, '33,5');
  act(() => (campoDe('Activa') as HTMLInputElement).click());
  act(() => (campoDe('Acme') as HTMLInputElement).click());
  act(() => boton('common.save')!.click());
  expect(guardados).toEqual([[{ nom: 'Eva', eda: 33.5, act: true }, { emp: ['e1'] }, false]]);
});

test('borrar va al final del cuerpo; en un registro nuevo, «Añadir registro» guarda y sigue', () => {
  const { borrados } = montar();
  act(() => boton('grid.deleteRecord:2')!.click());
  expect(borrados).toEqual([1]);
  render(null, document.body);
  const nuevo = montar({ nuevo: true });
  expect(boton('grid.deleteRecord:2')).toBeUndefined();
  act(() => boton('grid.addRecord')!.click());
  expect(nuevo.guardados).toEqual([[{}, {}, true]]);
});

test('quien solo lee ve los valores y nada que escriba', () => {
  montar({ puedeEditar: false });
  expect(document.querySelectorAll('input, select, textarea').length).toBe(0);
  expect(document.body.textContent).toContain('Ana');
  expect(document.body.textContent).toContain('view.unchecked');
  expect(boton('common.save')).toBeUndefined();
  expect(boton('grid.deleteRecord:2')).toBeUndefined();
  expect(boton('common.close')).toBeDefined();
});

const porNombre = (nombre: string) =>
  document.querySelector<HTMLButtonElement>(`button[aria-label="${nombre}"]`);

test('las flechas pasan al registro de al lado llevando lo cambiado, y se apagan sin vecino', () => {
  const idas: [number, Record<string, BasesValue>][] = [];
  montar({
    onIr: (paso, datos) => idas.push([paso, datos]),
    hayAnterior: false,
    haySiguiente: true,
  });
  expect(porNombre('record.previous')?.disabled).toBe(true);
  escribir(campoDe('Nombre') as HTMLInputElement, 'Eva');
  act(() => porNombre('record.next')!.click());
  expect(idas).toEqual([[1, { nom: 'Eva' }]]);
  // Un valor que no encaja no deja pasar: se corrige antes.
  escribir(campoDe('Edad') as HTMLInputElement, 'treinta');
  act(() => porNombre('record.next')!.click());
  expect(idas.length).toBe(1);
});

test('duplicar es de quien escribe y no se ofrece en un registro recién creado', () => {
  const copias: number[] = [];
  montar({ onDuplicar: () => copias.push(1) });
  act(() => boton('record.duplicate')!.click());
  expect(copias).toEqual([1]);
  render(null, document.body);
  montar({ onDuplicar: () => copias.push(1), nuevo: true });
  expect(boton('record.duplicate')).toBeUndefined();
  render(null, document.body);
  montar({ onDuplicar: () => copias.push(1), puedeEditar: false });
  expect(boton('record.duplicate')).toBeUndefined();
});
