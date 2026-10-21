/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Crear una regla para este remitente»: el modal se abre con el asistente en su
 * primer paso y el remitente ya escrito. La carga de la lista ponía el paso a null
 * DESPUÉS del efecto que lo abría, así que se veía la lista y el remitente se perdía
 * al pulsar «Crear regla». Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';

if (typeof document === 'undefined') GlobalRegistrator.register();

globalThis.fetch = (async () =>
  new Response(JSON.stringify({ filters: [] }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })) as unknown as typeof fetch;

const { FiltersModal } = await import('./FiltersModal.js');

async function montar(initial?: { matchFrom: string }): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <FiltersModal locale="es" open mailboxes={[]} onClose={() => {}} initial={initial} />,
      host,
    );
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  return host;
}

/** El campo con el remitente (Preact pone `value` como propiedad, no atributo). */
function remitente(): HTMLInputElement | undefined {
  return [...document.querySelectorAll('input')].find((i) => i.value === 'ana@example.org');
}

test('con remitente, se abre el asistente relleno', async () => {
  await montar({ matchFrom: 'ana@example.org' });
  expect(remitente()).toBeDefined();
  document.body.innerHTML = '';
});

test('sin remitente, se abre la lista', async () => {
  await montar();
  expect(remitente()).toBeUndefined();
  expect(document.body.textContent).toContain('Crear regla');
  document.body.innerHTML = '';
});
