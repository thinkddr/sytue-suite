/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { Balance, LibroDiario } from './Libro.js';
import { rangoDePeriodo } from './periodo.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const original = globalThis.fetch;
let host: HTMLDivElement;
let pedidas: string[] = [];
// Las consultas con `desde` contestan 500: un período que falla.
let fallarConPeriodo = false;
beforeEach(() => {
  pedidas = [];
  fallarConPeriodo = false;
  host = document.createElement('div');
  document.body.appendChild(host);
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'https://erp.example.test');
    pedidas.push(url.pathname + url.search);
    if (fallarConPeriodo && url.searchParams.has('desde')) {
      return new Response(JSON.stringify({ error: { message: 'Caído' } }), { status: 500 });
    }
    const cuerpo =
      url.pathname === '/api/balance'
        ? { cuentas: [], total_debe: 0, total_haber: 0 }
        : { asientos: [] };
    return new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = original;
});
async function esperar(condicion: () => boolean): Promise<void> {
  for (let i = 0; i < 40; i++) {
    if (condicion()) return;
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
  expect(condicion()).toBe(true);
}
async function elegirPeriodo(valor: string): Promise<void> {
  const sel = document.querySelector<HTMLSelectElement>(
    `select[aria-label="${t('ledger.period')}"]`,
  );
  if (!sel) throw new Error('No hay selector de período');
  sel.value = valor;
  await act(async () => {
    sel.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

// VISTO EN ROJO: sin pasar `rangoDePeriodo(periodo)` a la consulta, la segunda petición
// sale igual que la primera; sin `periodo` en las dependencias, ni siquiera sale.
test('el libro se acota al período elegido y lo dice cuando no hay nada', async () => {
  await act(async () => render(<LibroDiario locale="es" t={t} />, host));
  await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
  await elegirPeriodo('mes');
  const { desde, hasta } = rangoDePeriodo('mes');
  await esperar(() => document.body.textContent?.includes(t('ledger.emptyPeriod')) === true);
  expect(pedidas).toEqual([
    '/api/libro?limite=50',
    `/api/libro?limite=50&desde=${desde}&hasta=${hasta}`,
  ]);
});

test('el sumas y saldos también', async () => {
  await act(async () => render(<Balance locale="es" t={t} />, host));
  await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
  await elegirPeriodo('anioAnterior');
  const { desde, hasta } = rangoDePeriodo('anioAnterior');
  await esperar(() => pedidas.length === 2);
  expect(pedidas).toEqual(['/api/balance', `/api/balance?desde=${desde}&hasta=${hasta}`]);
});

function selector(): HTMLSelectElement | null {
  return document.querySelector<HTMLSelectElement>(`select[aria-label="${t('ledger.period')}"]`);
}

// VISTO EN ROJO: devolviendo solo la rueda mientras carga (o solo el error), el
// `<select>` se desmonta al cambiar de período: el foco cae al `body` y, con un 500,
// el selector desaparece y no hay forma de volver a «Todo».
for (const [nombre, Pantalla] of [
  ['libro', LibroDiario],
  ['balance', Balance],
] as const) {
  test(`${nombre}: el selector sigue ahí, con el foco, al cambiar y si el período falla`, async () => {
    fallarConPeriodo = true;
    await act(async () => render(<Pantalla locale="es" t={t} />, host));
    await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
    const antes = selector();
    antes?.focus();
    expect(document.activeElement).toBe(antes);
    await elegirPeriodo('mes');
    expect(selector()).toBe(antes);
    expect(document.activeElement).toBe(antes);
    await esperar(() => document.querySelector('[role="alert"]') !== null);
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('Caído');
    expect(selector()).toBe(antes);
    expect(document.activeElement).toBe(antes);
    fallarConPeriodo = false;
    await elegirPeriodo('todo');
    await esperar(() => document.body.textContent?.includes(t('ledger.empty')) === true);
    expect(document.querySelector('[role="alert"]')).toBeNull();
  });
}
