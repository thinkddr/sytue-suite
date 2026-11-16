/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel de actividad cuenta la traza con nombres y pide la página siguiente desde la
 * última entrada que se ve; el diálogo de descripción solo guarda si ha cambiado.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { VNode } from 'preact';
import type { BasesAuditEntry } from '@sytue/api-clients/bases';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import { PAGINA_ACTIVIDAD } from '../lib/actividad.js';
import { DialogoDescripcion, PanelActividad } from './PanelActividad.js';

// El DOM de pega solo mientras corre este fichero: happy-dom cambia también `fetch`,
// `Request` y `Response` globales, y los tests del BFF de este paquete usan los de Bun.
let registrado = false;
let fetchOriginal: typeof fetch;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register({ url: 'https://bases.sytue.test/' });
    registrado = true;
  }
  fetchOriginal = globalThis.fetch;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

let host: HTMLElement;
afterEach(() => {
  act(() => render(null, host));
  host.remove();
  globalThis.fetch = fetchOriginal;
});
function montar(v: VNode): void {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(v, host));
}
async function esperar(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 50 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
}
const t = traductor('es');

const entrada = (id: number): BasesAuditEntry => ({
  id,
  table_id: 't1',
  actor: 'ana',
  action: 'record.update',
  at: '2026-11-12T10:00:00Z',
});

test('cuenta la traza con nombres y «Cargar más» pide desde la última', async () => {
  const pedidas: string[] = [];
  const primera = Array.from({ length: PAGINA_ACTIVIDAD }, (_, i) => entrada(500 - i));
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    pedidas.push(url.pathname + url.search);
    let cuerpo: unknown = [];
    if (url.pathname.endsWith('/permissions'))
      cuerpo = [
        {
          id: 'p',
          principal_kind: 'user',
          principal_id: 'ana',
          principal_label: 'Ana Ruiz',
          role: 'editor',
          created_by: 'yo',
          created_at: '',
        },
      ];
    else if (url.pathname.endsWith('/audit'))
      cuerpo = url.searchParams.get('before') ? [{ ...entrada(7), actor: 'yo' }] : primera;
    return new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;

  montar(
    <PanelActividad
      t={t}
      locale="es"
      baseId="b1"
      nombre="Ventas"
      yo="yo"
      tablas={[{ id: 't1', base_id: 'b1', name: 'Pedidos', position: 0, fields: [] }]}
      onCerrar={() => {}}
    />,
  );
  const lista = () => document.querySelector(`ol[aria-label="${t('activity.listLabel')}"]`);
  await esperar(() => (lista()?.textContent ?? '').includes('Ana Ruiz'));
  expect(lista()?.children.length).toBe(PAGINA_ACTIVIDAD);
  expect(lista()?.textContent).toContain('Editó un registro');
  expect(lista()?.textContent).toContain('Pedidos');
  // Nunca el `sub` de nadie.
  expect(lista()?.textContent).not.toContain('ana ·');

  const mas = [...document.querySelectorAll('button')].find(
    (b) => b.textContent === t('activity.more'),
  )!;
  expect(mas).toBeTruthy();
  await act(async () => mas.click());
  await esperar(() => (lista()?.children.length ?? 0) > PAGINA_ACTIVIDAD);
  expect(pedidas).toContain(`/bases/api/bases/b1/audit?before=451&limit=${PAGINA_ACTIVIDAD}`);
  expect(lista()?.lastElementChild?.textContent).toContain('Tú');
  // La última página vino corta: ya no hay más.
  expect(
    [...document.querySelectorAll('button')].some((b) => b.textContent === t('activity.more')),
  ).toBe(false);
});

test('sin actividad lo dice, en vez de una lista vacía', async () => {
  globalThis.fetch = (async () =>
    new Response('[]', {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as unknown as typeof fetch;
  montar(
    <PanelActividad
      t={t}
      locale="es"
      baseId="b1"
      nombre="Ventas"
      yo="yo"
      tablas={[]}
      onCerrar={() => {}}
    />,
  );
  await esperar(() => (document.body.textContent ?? '').includes(t('activity.empty')));
  expect(document.body.textContent).toContain(t('activity.empty'));
});

test('la descripción solo se guarda si cambia, y recortada', () => {
  const guardadas: string[] = [];
  montar(
    <DialogoDescripcion
      t={t}
      valor="Clientes y pedidos"
      ocupado={false}
      error=""
      onGuardar={(d) => guardadas.push(d)}
      onCerrar={() => {}}
    />,
  );
  const area = document.getElementById('base-descripcion') as HTMLTextAreaElement;
  expect(area.value).toBe('Clientes y pedidos');
  const guardar = [...document.querySelectorAll('button')].find(
    (b) => b.textContent === t('common.save'),
  )!;
  expect(guardar.disabled).toBe(true);
  act(() => {
    area.value = '  Clientes, pedidos y facturas  ';
    area.dispatchEvent(new Event('input', { bubbles: true }));
  });
  expect(guardar.disabled).toBe(false);
  act(() => guardar.click());
  expect(guardadas).toEqual(['Clientes, pedidos y facturas']);
});
