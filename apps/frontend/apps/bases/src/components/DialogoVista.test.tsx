/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El diálogo de vista, al editar: sale con lo guardado, el tipo no se toca y lo que se
 * envía es la configuración nueva con el mismo tipo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import type { BasesField, BasesView, BasesViewConfig } from '@sytue/api-clients/bases';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import { DialogoVista } from './DialogoVista.js';

// El DOM de pega solo mientras corre este fichero: happy-dom cambia también `fetch`,
// `Request` y `Response` globales, y los tests del BFF de este paquete usan los de Bun.
let registrado = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registrado = true;
  }
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});
function montar(v: VNode): void {
  host = document.createElement('div');
  document.body.appendChild(host);
  render(v, host);
}
const t = traductor('es');
const num: BasesField = { id: 'n', table_id: 't', name: 'Importe', kind: 'number', position: 0 };
const txt: BasesField = { id: 'x', table_id: 't', name: 'Cliente', kind: 'text', position: 1 };
const vista: BasesView = {
  id: 'v1',
  table_id: 't',
  name: 'Grandes',
  kind: 'grid',
  config: { filters: [{ field: 'n', op: 'gt', value: 20 }], sort: [{ field: 'x', dir: 'desc' }] },
  created_by: 'ana',
};
const porEtiqueta = <E extends Element>(etiqueta: string): E =>
  document.querySelector<E>(`[aria-label="${etiqueta}"]`)!;
const boton = (texto: string): HTMLButtonElement =>
  Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === texto)!;

test('editar una vista: relleno con lo guardado, tipo fijo y se guarda la configuración nueva', async () => {
  const enviado: { nombre: string; tipo: string; cfg: BasesViewConfig }[] = [];
  montar(
    <DialogoVista
      t={t}
      campos={[num, txt]}
      error=""
      vista={vista}
      onCrear={(nombre, tipo, cfg) => enviado.push({ nombre, tipo, cfg })}
      onCerrar={() => undefined}
    />,
  );
  expect(document.body.textContent).toContain(t('view.edit'));
  expect(document.querySelector<HTMLInputElement>('#vista-nombre')!.value).toBe('Grandes');
  expect(document.querySelector<HTMLSelectElement>('#vista-tipo')!.disabled).toBe(true);
  expect(document.body.textContent).toContain(t('view.kindFixed'));
  expect(porEtiqueta<HTMLSelectElement>(t('view.filterField')).value).toBe('n');
  expect(porEtiqueta<HTMLSelectElement>(t('view.filterOp')).value).toBe('gt');
  const valor = porEtiqueta<HTMLInputElement>(t('view.filterValue'));
  expect(valor.value).toBe('20');
  expect(porEtiqueta<HTMLSelectElement>(t('view.sortDir')).value).toBe('desc');

  valor.value = '35';
  valor.dispatchEvent(new Event('input', { bubbles: true }));
  await Promise.resolve();
  const guardar = boton(t('view.save'));
  expect(guardar.disabled).toBe(false);
  document
    .querySelector<HTMLFormElement>('#form-vista')!
    .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  expect(enviado).toEqual([
    {
      nombre: 'Grandes',
      tipo: 'grid',
      cfg: { filters: [{ field: 'n', op: 'gt', value: 35 }], sort: [{ field: 'x', dir: 'desc' }] },
    },
  ]);
});

test('sin vista el diálogo crea: título de añadir y el tipo se puede elegir', () => {
  montar(
    <DialogoVista
      t={t}
      campos={[num]}
      error=""
      onCrear={() => undefined}
      onCerrar={() => undefined}
    />,
  );
  expect(document.body.textContent).toContain(t('view.add'));
  expect(document.querySelector<HTMLSelectElement>('#vista-tipo')!.disabled).toBe(false);
  expect(document.querySelector<HTMLInputElement>('#vista-nombre')!.value).toBe('');
});

test('crear una galería: se elige el tipo, dice qué es la portada y se envía con su orden', async () => {
  const enviado: { nombre: string; tipo: string; cfg: BasesViewConfig }[] = [];
  montar(
    <DialogoVista
      t={t}
      campos={[num, txt]}
      error=""
      onCrear={(nombre, tipo, cfg) => enviado.push({ nombre, tipo, cfg })}
      onCerrar={() => undefined}
    />,
  );
  const tipo = document.querySelector<HTMLSelectElement>('#vista-tipo')!;
  expect(Array.from(tipo.options).map((o) => o.value)).toContain('gallery');
  tipo.value = 'gallery';
  tipo.dispatchEvent(new Event('change', { bubbles: true }));
  const nombre = document.querySelector<HTMLInputElement>('#vista-nombre')!;
  nombre.value = 'Fotos';
  nombre.dispatchEvent(new Event('input', { bubbles: true }));
  await Promise.resolve();
  expect(document.body.textContent).toContain(t('view.galleryHelp'));
  const orden = porEtiqueta<HTMLSelectElement>(t('view.sortField'));
  orden.value = 'x';
  orden.dispatchEvent(new Event('change', { bubbles: true }));
  await Promise.resolve();
  document
    .querySelector<HTMLFormElement>('#form-vista')!
    .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  expect(enviado).toEqual([
    { nombre: 'Fotos', tipo: 'gallery', cfg: { sort: [{ field: 'x', dir: 'asc' }] } },
  ]);
});
