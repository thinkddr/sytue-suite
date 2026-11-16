/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA REVISIÓN DE SEGURIDAD, CON LA PÁGINA ENTERA DE TARJETAS Y EL SERVIDOR SIMULADO.
 *
 * - con la verificación apagada sale el aviso con «Turn on», que lleva a #seguridad,
 *   y esa ancla existe en la página;
 * - con todo en orden, una sola línea positiva;
 * - montada junto a las demás tarjetas, cada recurso se pide UNA vez (antes
 *   `/api/me` salía tres veces, y la revisión habría sumado cinco peticiones más).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useT as traductor } from '../i18n/index.js';
import { en } from '../i18n/messages/en.js';
import { olvidarLecturas } from '../lib/primeraLectura.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.test/account/' });
const fetchOriginal = globalThis.fetch;

const { RevisionSeguridad } = await import('./RevisionSeguridad.js');
const { PerfilCard } = await import('./PerfilCard.js');
const { SeguridadCard } = await import('./SeguridadCard.js');
const { RecuperacionCard } = await import('./RecuperacionCard.js');
const { SesionesCard } = await import('./SesionesCard.js');
const { DispositivosCard } = await import('./DispositivosCard.js');
const { AppPasswordsCard } = await import('./AppPasswordsCard.js');

const t = traductor('en');
let pedidas: string[] = [];
let me: Record<string, unknown> = {};

beforeEach(() => {
  pedidas = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const clave = `${init?.method ?? 'GET'} ${String(input)}`;
    pedidas.push(clave);
    const cuerpos: Record<string, unknown> = {
      'GET /api/me': me,
      'GET /api/passkeys': { credentials: [] },
      'GET /api/sessions': { sessions: [] },
      'GET /api/devices': { devices: [] },
      'GET /api/app-passwords': { app_passwords: [] },
    };
    const cuerpo = cuerpos[clave];
    return new Response(JSON.stringify(cuerpo ?? {}), {
      status: cuerpo === undefined ? 404 : 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
});

afterEach(() => {
  act(() => render(null, document.body));
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
  olvidarLecturas();
});

afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

async function esperar(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 50 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 5));
    });
  }
  expect(cond()).toBe(true);
}

async function montarPagina(): Promise<void> {
  await act(async () => {
    render(
      <div>
        <RevisionSeguridad t={t} />
        <PerfilCard t={t} nombreInicial="Ana" email="ana@example.eu" idBaseUrl="https://id" />
        <SesionesCard t={t} locale="en" />
        <SeguridadCard t={t} />
        <RecuperacionCard t={t} />
        <DispositivosCard t={t} locale="en" />
        <AppPasswordsCard t={t} locale="en" />
      </div>,
      document.body,
    );
  });
}

const revision = (): Element | null => document.getElementById('revision');

describe('revisión de seguridad', () => {
  test('sin verificación en dos pasos: aviso con «Turn on» que lleva a #seguridad', async () => {
    me = { mfa_enabled: false, recovery_email: 'otra@example.eu' };
    await montarPagina();
    await esperar(() => revision()?.querySelector('[data-revision="dos_pasos"]') != null);
    const fila = revision()!.querySelector('[data-revision="dos_pasos"]')!;
    expect(fila.textContent).toContain(en['checkup.dos_pasos']);
    const enlace = fila.querySelector('a')!;
    expect(enlace.textContent?.trim()).toBe(en['checkup.turn_on']);
    expect(enlace.getAttribute('href')).toBe('#seguridad');
    expect(document.getElementById('seguridad')).not.toBeNull();
    // Lo que está bien no se lista: solo el aviso.
    expect(revision()!.querySelectorAll('li[data-revision]').length).toBe(1);
  });

  test('todo en orden: una sola línea positiva', async () => {
    me = { mfa_enabled: true, phone: '+34600000000' };
    await montarPagina();
    await esperar(() => revision()?.querySelector('[data-revision="ok"]') != null);
    expect(revision()!.textContent).toContain(en['checkup.all_good']);
    expect(revision()!.querySelectorAll('li').length).toBe(0);
  });

  test('una sola petición por recurso al cargar la página', async () => {
    me = { mfa_enabled: true, recovery_email: 'otra@example.eu' };
    await montarPagina();
    await esperar(() => revision()?.querySelector('[data-revision="ok"]') != null);
    const gets = pedidas.filter((p) => p.startsWith('GET /api/'));
    const veces = Object.fromEntries(gets.map((g) => [g, gets.filter((x) => x === g).length]));
    expect(veces).toEqual({
      'GET /api/me': 1,
      'GET /api/passkeys': 1,
      'GET /api/sessions': 1,
      'GET /api/devices': 1,
      'GET /api/app-passwords': 1,
    });
  });
});
