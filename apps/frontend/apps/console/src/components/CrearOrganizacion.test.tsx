/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El alta de organizaciones de la consola de plataforma: sin elevación viva no se
 * ofrece, y un rechazo de Sytue ID se lee en el propio formulario —un 403 aquí no
 * recarga, a diferencia del resto del panel—. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { PlatformActivation } from '../lib/platform.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { CrearOrganizacion } = await import('./CrearOrganizacion.js');
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

const elevacion = (effective_status: PlatformActivation['effective_status']) =>
  ({ id: 'a', effective_status, status: 'active' }) as PlatformActivation;

let llamadas: { url: string; body: Record<string, unknown> }[] = [];
function respuestas(...rs: Response[]) {
  llamadas = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    llamadas.push({ url, body: JSON.parse(String(init?.body ?? '{}')) });
    return rs.shift() ?? new Response(null, { status: 500 });
  }) as unknown as typeof fetch;
}
const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

let host: HTMLElement | null = null;
afterEach(async () => {
  if (host) await act(async () => render(null, host!));
  document.body.innerHTML = '';
});

async function montar(activation: PlatformActivation | null) {
  if (host) await act(async () => render(null, host!));
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(<CrearOrganizacion t={t} activation={activation} />, host!);
  });
}
const boton = (texto: string) =>
  [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto);
async function pulsar(texto: string) {
  const b = boton(texto);
  if (!b) throw new Error(`no hay botón «${texto}»`);
  await act(async () => {
    b.click();
    await new Promise((r) => setTimeout(r, 0));
  });
}
async function escribir(id: string, valor: string) {
  const i = document.getElementById(id) as HTMLInputElement;
  await act(async () => {
    i.value = valor;
    i.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

test('sin elevación viva no se ofrece el alta', async () => {
  for (const a of [null, elevacion('expired'), elevacion('revoked')]) {
    await montar(a);
    expect(boton(t('platform.create.open'))).toBeUndefined();
  }
  await montar(elevacion('active'));
  expect(boton(t('platform.create.open'))).toBeDefined();
});

test('un 403 se enseña como error y conserva lo escrito', async () => {
  respuestas(json(403, { code: 'activation_required', message: 'Elevate first.' }));
  await montar(elevacion('active'));
  await pulsar(t('platform.create.open'));
  await escribir('alta-slug', 'pruebas');
  await pulsar(t('platform.create.submit'));
  expect(llamadas).toHaveLength(1);
  expect(document.querySelector('[role=alert]')?.textContent).toBe(
    t('platform.create.error.forbidden'),
  );
  expect((document.getElementById('alta-slug') as HTMLInputElement).value).toBe('pruebas');
});

test('con plazas de prueba, crea la organización y después las concede', async () => {
  respuestas(json(201, { id: 'org-1', slug: 'pruebas' }), json(201, {}));
  await montar(elevacion('active'));
  await pulsar(t('platform.create.open'));
  await escribir('alta-slug', 'pruebas');
  await escribir('alta-propietaria', 'auditoria@example.org');
  await escribir('alta-prueba', '5');
  await escribir('alta-caduca', '2026-12-31');
  await escribir('alta-motivo', 'Organización de pruebas');
  await pulsar(t('platform.create.submit'));
  expect(llamadas.map((l) => l.url)).toEqual([
    '/api/platform/orgs',
    '/api/platform/orgs/org-1/seat-grants',
  ]);
  expect(llamadas[0]!.body).toEqual({
    slug: 'pruebas',
    name: '',
    owner_email: 'auditoria@example.org',
    seats: 1,
  });
  expect(llamadas[1]!.body).toEqual({
    seats: 5,
    kind: 'trial',
    reason: 'Organización de pruebas',
    expires_at: '2026-12-31T00:00:00Z',
  });
  const estado = document.querySelector('[role=status]')?.textContent ?? '';
  expect(estado).toContain(t('platform.create.done', { slug: 'pruebas' }));
  expect(estado).toContain(t('platform.create.doneTrial', { count: 5 }));
  expect(document.querySelector('[role=alert]')).toBeNull();
});
