/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel del registro RGPD contra un BFF falso: lo que se pinta de cada solicitud
 * (plazo vencido incluido), que el alta de alguien de fuera manda su correo y no una
 * cuenta, y que la supresión no se puede pulsar hasta escribir el correo de la
 * persona, ni sin decir antes qué borra y qué no. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { DetalleRGPD, ListadoRGPD, SolicitudRGPD } from '../lib/privacidad.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/admin/';

const { default: PrivacyPanel } = await import('./PrivacyPanel.js');

const vencida: SolicitudRGPD = {
  id: 'sol-1',
  subject_user_id: 'u-pablo',
  subject_email: 'pablo@acme.test',
  subject_name: 'Pablo',
  kind: 'erasure',
  channel: 'email',
  received_at: '2026-09-01T09:00:00Z',
  due_at: '2026-10-01T09:00:00Z',
  extended: false,
  identity_verified: true,
  status: 'in_progress',
  created_at: '2026-09-01T09:00:00Z',
  updated_at: '2026-09-01T09:00:00Z',
};
const listado: ListadoRGPD = { requests: [vencida], summary: { open: 1, overdue: 1 } };

let peticiones: { url: string; method: string; body: unknown }[] = [];
const original = globalThis.fetch;

function bffFalso(detalle: DetalleRGPD): void {
  peticiones = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    peticiones.push({ url, method, body: init?.body ? JSON.parse(String(init.body)) : null });
    if (method === 'GET' && url.startsWith('/api/admin/privacy/requests/')) {
      return new Response(JSON.stringify(detalle), { status: 200 });
    }
    if (method === 'GET') return new Response(JSON.stringify(listado), { status: 200 });
    return new Response(JSON.stringify({ request: vencida }), { status: 201 });
  }) as typeof fetch;
}

afterEach(() => {
  globalThis.fetch = original;
  render(null, document.body);
});

async function esperar(): Promise<void> {
  for (let i = 0; i < 5; i++) await act(async () => await new Promise((r) => setTimeout(r, 0)));
}

function boton(texto: string): HTMLButtonElement {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent?.trim() === texto);
  if (!b) throw new Error(`sin botón «${texto}»`);
  return b as HTMLButtonElement;
}

function escribir(el: HTMLInputElement, valor: string): void {
  el.value = valor;
  el.dispatchEvent(new Event('input', { bubbles: true }));
}

test('pinta la solicitud con su plazo vencido y el recuento', async () => {
  bffFalso({ request: vencida, events: [], erasure: { available: true } });
  await act(async () => {
    render(<PrivacyPanel locale="es" personas={[]} inicial={listado} />, document.body);
  });
  const texto = document.body.textContent ?? '';
  expect(texto).toContain('Pablo');
  expect(texto).toContain('Supresión');
  expect(texto).toContain('Vencida');
  expect(texto).toContain('En curso');
  // Con las abiertas ya en la página, no se pide nada al montar.
  expect(peticiones).toEqual([]);
});

test('el alta de alguien de fuera manda su correo, no una cuenta', async () => {
  bffFalso({ request: vencida, events: [], erasure: { available: false } });
  await act(async () => {
    render(<PrivacyPanel locale="en" personas={[]} inicial={listado} />, document.body);
  });
  await act(async () => boton('Register a request').click());
  const correo = document.getElementById('rgpd-correo') as HTMLInputElement;
  await act(async () => escribir(correo, 'no-es-correo'));
  const registrar = [...document.querySelectorAll('button')].filter(
    (b) => b.textContent?.trim() === 'Register a request',
  );
  await act(async () => registrar.at(-1)?.click());
  expect(document.body.textContent).toContain('Enter a valid email address.');
  expect(peticiones.filter((p) => p.method === 'POST')).toEqual([]);

  await act(async () => escribir(correo, 'cliente@fuera.test'));
  await act(async () => registrar.at(-1)?.click());
  await esperar();
  const post = peticiones.find((p) => p.method === 'POST');
  expect(post?.url).toBe('/api/admin/privacy/requests');
  expect(post?.body).toMatchObject({
    subject_email: 'cliente@fuera.test',
    kind: 'access',
    channel: 'email',
  });
  const cuerpo = (post?.body ?? {}) as Record<string, unknown>;
  expect(cuerpo.user_id).toBeUndefined();
});

test('la supresión dice qué borra y no se pulsa hasta escribir el correo', async () => {
  bffFalso({
    request: vencida,
    events: [{ id: 1, at: '2026-09-01T09:00:00Z', action: 'created' }],
    erasure: { available: true },
  });
  await act(async () => {
    render(<PrivacyPanel locale="en" personas={[]} inicial={listado} />, document.body);
  });
  // La fila se abre por la persona (su nombre y, debajo, su correo).
  const fila = [...document.querySelectorAll('button')].find((b) =>
    b.textContent?.startsWith('Pablo'),
  ) as HTMLButtonElement;
  await act(async () => fila.click());
  await esperar();
  const texto = document.body.textContent ?? '';
  expect(texto).toContain('stays with the organisation');
  expect(texto).toContain('Registered');
  const suprimir = [...document.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === 'Erase the account',
  ) as HTMLButtonElement;
  expect(suprimir.disabled).toBe(true);
  await act(async () =>
    escribir(document.getElementById('rgpd-confirmar') as HTMLInputElement, 'PABLO@acme.test'),
  );
  expect(suprimir.disabled).toBe(false);
  await act(async () => suprimir.click());
  await esperar();
  const post = peticiones.find((p) => p.url.endsWith('/erase'));
  expect(post?.body).toEqual({ confirm: 'PABLO@acme.test' });
});
