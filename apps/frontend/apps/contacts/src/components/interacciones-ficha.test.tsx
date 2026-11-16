/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Interacciones recientes» de la ficha: enseña lo que da el Hub con enlace a su app,
 * distingue «no hay» de «no está disponible», y no aparece si no hay correo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { SearchResult } from '@sytue/api-clients';
import { InteraccionesRecientes } from './InteraccionesRecientes.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const hosts: HTMLElement[] = [];
afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

async function pintar(
  emails: string[],
  cargar: (e: readonly string[]) => Promise<SearchResult[]>,
): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  await act(async () => {
    render(<InteraccionesRecientes t={t} locale="es" emails={emails} cargar={cargar} />, host);
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  return host;
}

test('lista los correos y eventos con enlace a su app', async () => {
  let pedidos: readonly string[] = [];
  const host = await pintar(['ana@x.eu', 'ana@casa.eu'], async (e) => {
    pedidos = e;
    return [
      { source: 'mail', id: '1', title: 'Presupuesto', url: '/mail/m/1', timestamp: Date.now() },
      { source: 'calendar', id: 'e', title: 'Reunión', url: 'javascript:alert(1)' },
    ] as SearchResult[];
  });
  expect(pedidos).toEqual(['ana@x.eu', 'ana@casa.eu']);
  expect(host.textContent).toContain('detail.interactionsTitle');
  const enlaces = [...host.querySelectorAll('a')];
  expect(
    enlaces.map((a) => [a.textContent?.includes('Presupuesto'), a.getAttribute('href')]),
  ).toEqual([[true, '/mail/m/1']]);
  // El `javascript:` no se convierte en enlace, pero la fila se ve.
  expect(host.textContent).toContain('Reunión');
});

test('«no hay» y «no disponible» se dicen distinto', async () => {
  expect((await pintar(['a@x.eu'], async () => [])).textContent).toContain(
    'detail.interactionsEmpty',
  );
  expect((await pintar(['a@x.eu'], () => Promise.reject(new Error('hub')))).textContent).toContain(
    'detail.interactionsError',
  );
});

test('sin correo no hay sección ni búsqueda', async () => {
  let llamado = false;
  const host = await pintar([], async () => {
    llamado = true;
    return [];
  });
  expect(llamado).toBe(false);
  expect(host.innerHTML).toBe('');
});
