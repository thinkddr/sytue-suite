/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LO NUEVO DE MI CUENTA, CON EL SERVIDOR SIMULADO.
 *
 * - «Actividad reciente»: traduce la clase pública de Sytue ID (y no se come una
 *   clase que todavía no conoce), enseña diez y el resto tras «Ver todo», y con el
 *   servicio caído lo dice y ofrece reintentar.
 * - Las passkeys dicen cuándo se añadieron y cuándo se usaron, y se renombran en
 *   línea: Intro guarda con un PATCH del nombre recortado, Escape no manda nada.
 * - La contraseña de aplicación recién creada se copia con un botón.
 * - Imprimir los códigos marca el documento SOLO mientras dura la impresión.
 *
 * Vistos en ROJO quitando cada pieza: sin el respaldo de `rotuloDeSuceso` la clase
 * desconocida salía como la clave cruda; sin el `trim()` el PATCH llevaba los
 * espacios; sin quitar la marca, el siguiente Ctrl+P salía recortado.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { act } from 'preact/test-utils';
import { useT as traductor } from '../i18n/index.js';
import { en } from '../i18n/messages/en.js';
import { olvidarLecturas } from '../lib/primeraLectura.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.test/account/' });
const fetchOriginal = globalThis.fetch;

const { ActividadCard, A_LA_VISTA } = await import('./ActividadCard.js');
const { SeguridadCard } = await import('./SeguridadCard.js');
const { AppPasswordsCard } = await import('./AppPasswordsCard.js');
const { imprimirCodigos } = await import('../lib/codigosRecuperacion.js');

const t = traductor('en');

type Respuestas = Record<string, [number, unknown]>;
let respuestas: Respuestas = {};
let pedidas: { clave: string; cuerpo: string }[] = [];

beforeEach(() => {
  pedidas = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const clave = `${init?.method ?? 'GET'} ${String(input)}`;
    pedidas.push({ clave, cuerpo: typeof init?.body === 'string' ? init.body : '' });
    const [estado, cuerpo] = respuestas[clave] ?? [404, { error: 'not_found' }];
    return new Response(JSON.stringify(cuerpo), {
      status: estado,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
});

afterEach(() => {
  act(() => render(null, document.body));
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
  respuestas = {};
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

async function montar(arbol: ComponentChild): Promise<void> {
  await act(async () => {
    render(arbol, document.body);
  });
}

const boton = (nombre: string): HTMLButtonElement | undefined =>
  [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.getAttribute('aria-label') === nombre || b.textContent?.trim() === nombre,
  );

async function pulsar(nombre: string): Promise<void> {
  const b = boton(nombre);
  expect(b, nombre).toBeDefined();
  await act(async () => {
    b!.click();
  });
}

const texto = (): string => document.body.textContent ?? '';
const hace = (min: number): string => new Date(Date.now() - min * 60_000).toISOString();

describe('actividad reciente', () => {
  test('traduce cada clase, y una que no conoce sale como cambio en la cuenta', async () => {
    respuestas['GET /api/activity'] = [
      200,
      {
        days: 90,
        events: [
          { id: 3, kind: 'password_changed', at: hace(5), ip: '203.0.113.7' },
          { id: 2, kind: 'signin', at: hace(60), ip: '198.51.100.2' },
          { id: 1, kind: 'algo_nuevo', at: hace(120) },
        ],
      },
    ];
    await montar(<ActividadCard t={t} locale="en" />);
    await esperar(() => texto().includes(en['activity.kind.password_changed']));
    expect(texto()).toContain(en['activity.kind.signin']);
    expect(texto()).toContain(en['activity.kind.other']);
    expect(texto()).not.toContain('algo_nuevo');
    expect(texto()).toContain('203.0.113.7');
    // Con algo que mirar, qué hacer si no se reconoce.
    expect(texto()).toContain(en['activity.not_you']);
    expect(document.querySelector('a[href="#seguridad"]')).not.toBeNull();
  });

  test(`enseña ${A_LA_VISTA} y el resto tras «Ver todo»`, async () => {
    const eventos = Array.from({ length: A_LA_VISTA + 5 }, (_, i) => ({
      id: i + 1,
      kind: 'signin',
      at: hace(i + 1),
    }));
    respuestas['GET /api/activity'] = [200, { days: 90, events: eventos }];
    await montar(<ActividadCard t={t} locale="en" />);
    const filas = () => document.querySelectorAll('#actividad li').length;
    await esperar(() => filas() === A_LA_VISTA);
    await pulsar(en['activity.more']);
    expect(filas()).toBe(A_LA_VISTA + 5);
    expect(boton(en['activity.more'])).toBeUndefined();
  });

  test('sin actividad lo dice con la ventana; con el servicio caído, reintentar', async () => {
    respuestas['GET /api/activity'] = [500, {}];
    await montar(<ActividadCard t={t} locale="en" />);
    await esperar(() => texto().includes(en['activity.failed']));
    respuestas['GET /api/activity'] = [200, { days: 30, events: [] }];
    await pulsar(en['common.retry']);
    await esperar(() => texto().includes(en['activity.empty'].replace('{days}', '30')));
  });
});

describe('passkeys', () => {
  const LLAVES = {
    credentials: [
      { id: 'k1', name: 'Passkey', created_at: '2026-10-03T10:00:00Z', last_used: hace(30) },
      { id: 'k2', name: '', created_at: '2026-10-04T10:00:00Z', last_used: null },
    ],
  };

  test('dicen cuándo se añadieron y cuándo se usaron', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: true }];
    respuestas['GET /api/passkeys'] = [200, LLAVES];
    await montar(<SeguridadCard t={t} locale="en" />);
    await esperar(() => texto().includes(en['security.passkeys.never_used']));
    // El formato exacto es el de `formatDate`/`formatTimeAgo` (en-GB en la suite).
    expect(texto()).toContain('Added on 3 Oct 2026');
    expect(texto()).toMatch(/Last used 30 min ago/);
  });

  test('se renombran en línea: Intro guarda el nombre recortado, Escape no manda nada', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: true }];
    respuestas['GET /api/passkeys'] = [200, LLAVES];
    respuestas['PATCH /api/passkeys/k1'] = [200, { id: 'k1', name: 'MacBook' }];
    await montar(<SeguridadCard t={t} locale="en" />);
    await esperar(() => boton('Rename Passkey') !== undefined);

    // Escape: se cierra sin pedir nada.
    await pulsar('Rename Passkey');
    const campo = () => document.getElementById('pk-renombrar-k1') as HTMLInputElement | null;
    expect(campo()).not.toBeNull();
    await act(async () => {
      campo()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    });
    expect(campo()).toBeNull();
    expect(pedidas.some((p) => p.clave.startsWith('PATCH'))).toBe(false);

    // Intro: PATCH con el nombre recortado y se relee la lista.
    await pulsar('Rename Passkey');
    await act(async () => {
      campo()!.value = '  MacBook  ';
      campo()!.dispatchEvent(new Event('input', { bubbles: true }));
    });
    const lecturas = pedidas.filter((p) => p.clave === 'GET /api/passkeys').length;
    await act(async () => {
      campo()!.form!.requestSubmit();
    });
    await esperar(() => pedidas.some((p) => p.clave === 'PATCH /api/passkeys/k1'));
    const patch = pedidas.find((p) => p.clave === 'PATCH /api/passkeys/k1')!;
    expect(JSON.parse(patch.cuerpo)).toEqual({ name: 'MacBook' });
    await esperar(() => pedidas.filter((p) => p.clave === 'GET /api/passkeys').length > lecturas);
  });

  test('si el servicio no deja renombrar, lo dice y el campo sigue abierto', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: true }];
    respuestas['GET /api/passkeys'] = [200, LLAVES];
    respuestas['PATCH /api/passkeys/k1'] = [500, {}];
    await montar(<SeguridadCard t={t} locale="en" />);
    await esperar(() => boton('Rename Passkey') !== undefined);
    await pulsar('Rename Passkey');
    const campo = document.getElementById('pk-renombrar-k1') as HTMLInputElement;
    await act(async () => {
      campo.value = 'Otro';
      campo.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => {
      campo.form!.requestSubmit();
    });
    await esperar(() => texto().includes(en['security.passkeys.rename_failed']));
    expect(document.getElementById('pk-renombrar-k1')).not.toBeNull();
  });
});

describe('contraseña sugerida', () => {
  test('rellena el campo, la enseña en claro y vuelve a ocultarla al teclear encima', async () => {
    respuestas['GET /api/me'] = [
      200,
      { mfa_enabled: false, password_policy: { min_length: 12, require_complexity: true } },
    ];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    await montar(<SeguridadCard t={t} locale="en" />);
    await esperar(() => boton(en['security.password.suggest']) !== undefined);
    await pulsar(en['security.password.suggest']);
    const campo = document.getElementById('pwd-nueva') as HTMLInputElement;
    expect(campo.value).toMatch(/^[a-z2-9]{5}(-[a-z2-9]{5}){3}$/);
    expect(campo.type).toBe('text');
    expect(texto()).toContain(en['security.password.suggested']);
    await act(async () => {
      campo.value = campo.value + 'x';
      campo.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect((document.getElementById('pwd-nueva') as HTMLInputElement).type).toBe('password');
  });
});

describe('contraseña de aplicación', () => {
  test('la recién creada se copia con un botón', async () => {
    let copiado = '';
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: async (s: string) => void (copiado = s) },
    });
    respuestas['GET /api/app-passwords'] = [200, { app_passwords: [] }];
    respuestas['POST /api/app-passwords'] = [200, { label: 'Thunderbird', password: 'abcd efgh' }];
    await montar(<AppPasswordsCard t={t} locale="en" />);
    await esperar(() => boton(en['app_passwords.create']) !== undefined);
    await act(async () => {
      document.querySelector('form')!.requestSubmit();
    });
    await esperar(() => texto().includes('abcd efgh'));
    await pulsar(en['common.copy']);
    await esperar(() => texto().includes(en['common.copied']));
    expect(copiado).toBe('abcd efgh');
  });
});

describe('imprimir los códigos', () => {
  test('marca el documento solo mientras imprime', () => {
    let marcaAlImprimir: string | undefined;
    imprimirCodigos(() => {
      marcaAlImprimir = document.documentElement.dataset.imprimir;
      window.dispatchEvent(new Event('afterprint'));
    });
    expect(marcaAlImprimir).toBe('codigos');
    expect(document.documentElement.dataset.imprimir).toBeUndefined();
  });
});
