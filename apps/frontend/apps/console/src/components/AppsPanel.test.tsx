/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Estado de la sincronización» de una app SCIM conectada: una app sin pasadas y un
 * servicio que no contesta no pueden decirse igual. Antes los dos salían como «aún
 * no hay sincronizaciones», y un fallo se leía como una app recién conectada.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { CatalogApp, ConnectedInstance } from '../lib/gateway.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { default: AppsPanel } = await import('./AppsPanel.js');
const { es: messages } = await import('../i18n/messages/es.js');

const app: CatalogApp = {
  key: 'acme-hr',
  name: 'Acme RR. HH.',
  description: '',
  color: 'blue',
  iconUrl: '',
  identity: 'none',
  provisioning: 'scim',
  hasData: false,
  fields: [],
  tier: 'free',
};
const inst: ConnectedInstance = {
  id: 'i1',
  templateKey: 'acme-hr',
  name: 'Acme RR. HH.',
  status: 'active',
  hasSecrets: true,
};

function servidor(syncRuns: () => Response) {
  globalThis.fetch = (async (url: string) => {
    if (String(url).includes('/sync-runs')) return syncRuns();
    return new Response(JSON.stringify({}), { status: 200 });
  }) as typeof fetch;
}

const espera = () => act(() => new Promise<void>((r) => setTimeout(r, 0)));

async function pedirEstado(): Promise<HTMLElement> {
  const raiz = document.createElement('div');
  document.body.append(raiz);
  await act(() => {
    render(
      <AppsPanel locale="es" catalog={[app]} instances={[inst]} live productChoices={[]} />,
      raiz,
    );
  });
  const boton = [...raiz.querySelectorAll('button')].find(
    (b) => b.textContent === messages['apps.syncStatus'],
  );
  expect(boton).toBeDefined();
  await act(() => boton!.click());
  await espera();
  await espera();
  return raiz;
}

afterEach(() => {
  document.body.innerHTML = '';
});

test('sin pasadas registradas dice que aún no hay sincronizaciones', async () => {
  servidor(() => new Response(JSON.stringify({ sync_runs: [] }), { status: 200 }));
  const raiz = await pedirEstado();
  expect(raiz.textContent).toContain(messages['apps.runs.none']);
  expect(raiz.textContent).not.toContain(messages['apps.runs.failed']);
});

test('un fallo del servicio se dice como fallo, no como «aún no hay»', async () => {
  servidor(() => new Response(null, { status: 503 }));
  const raiz = await pedirEstado();
  expect(raiz.textContent).toContain(messages['apps.runs.failed']);
  expect(raiz.textContent).not.toContain(messages['apps.runs.none']);
});
