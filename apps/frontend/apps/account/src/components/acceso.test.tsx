/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «ACCESO Y RECUPERACIÓN»: CAMBIAR EL @USUARIO Y EL MÓVIL, CON EL SERVIDOR SIMULADO.
 *
 * - el @usuario nuevo va con la contraseña y, al cambiar, la tarjeta enseña el nuevo;
 *   cada negativa de Sytue ID se dice con su aviso (la reserva de 30 días, la cuenta
 *   personal);
 * - el móvil va en dos pasos: el código por SMS y la confirmación, y no se enseña
 *   como guardado hasta confirmar; una cuenta sin móvil lo ofrece para añadir;
 * - que el número ya es de otra cuenta se dice tras el código, y se vuelve a empezar.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useT as traductor } from '../i18n/index.js';
import { en } from '../i18n/messages/en.js';
import { olvidarLecturas } from '../lib/primeraLectura.js';
import { falloDeMovil, falloDeUsuario } from '../lib/accesoCuenta.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.test/account/' });
const fetchOriginal = globalThis.fetch;

const { RecuperacionCard } = await import('./RecuperacionCard.js');

const t = traductor('en');

type Respuestas = Record<string, [number, unknown]>;
let respuestas: Respuestas = {};
let pedidas: { clave: string; cuerpo: unknown }[] = [];

beforeEach(() => {
  pedidas = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const clave = `${init?.method ?? 'GET'} ${String(input)}`;
    pedidas.push({ clave, cuerpo: init?.body ? JSON.parse(String(init.body)) : undefined });
    const [estado, cuerpo] = respuestas[clave] ?? [404, { error: { code: 'not_found' } }];
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

function escribir(id: string, valor: string): void {
  const input = document.getElementById(id) as HTMLInputElement;
  input.value = valor;
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function enviar(seccion: string): Promise<void> {
  const form = document.querySelector(`form[data-seccion="${seccion}"]`) as HTMLFormElement;
  await act(async () => {
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  });
}

const texto = (): string => document.body.textContent ?? '';

async function montar(me: Record<string, unknown>): Promise<void> {
  respuestas['GET /api/me'] = [200, me];
  await act(async () => {
    render(<RecuperacionCard t={t} />, document.body);
  });
  await esperar(() => pedidas.some((p) => p.clave === 'GET /api/me'));
}

describe('@usuario', () => {
  test('cambia con la contraseña y la tarjeta enseña el nuevo', async () => {
    await montar({ username: 'ana', phone: '+34600000000' });
    await esperar(
      () => (document.getElementById('rec-usuario') as HTMLInputElement)?.value === '@ana',
    );
    respuestas['PUT /api/username'] = [200, { username: 'ana.nueva' }];
    await act(async () => {
      escribir('rec-password', 'secreta-larga');
      escribir('rec-usuario-nuevo', '@Ana.Nueva');
    });
    await enviar('usuario');
    await esperar(() => texto().includes(en['recovery.username.changed']));
    expect(pedidas.find((p) => p.clave === 'PUT /api/username')?.cuerpo).toEqual({
      username: '@Ana.Nueva',
      password: 'secreta-larga',
    });
    expect((document.getElementById('rec-usuario') as HTMLInputElement).value).toBe('@ana.nueva');
  });

  test('la reserva de 30 días y la cuenta personal se dicen con su aviso', async () => {
    for (const [code, clave] of [
      ['username_recent', 'recovery.username.recent'],
      ['username_is_address', 'recovery.username.is_address'],
    ] as const) {
      await montar({ username: 'ana' });
      respuestas['PUT /api/username'] = [409, { error: { code } }];
      await act(async () => {
        escribir('rec-password', 'secreta-larga');
        escribir('rec-usuario-nuevo', 'otra');
      });
      await enviar('usuario');
      await esperar(() => texto().includes(en[clave]));
      act(() => render(null, document.body));
      olvidarLecturas();
    }
  });
});

describe('móvil', () => {
  test('una cuenta sin móvil lo añade en dos pasos y no se da por guardado antes del código', async () => {
    await montar({ username: 'luis' });
    await esperar(() => texto().includes(en['recovery.phone.none']));
    respuestas['PUT /api/phone'] = [202, { pending: '+34611222333' }];
    respuestas['POST /api/phone'] = [200, { phone: '+34611222333' }];
    await act(async () => {
      escribir('rec-password', 'secreta-larga');
      escribir('rec-movil-nuevo', '+34 611 222 333');
    });
    await enviar('movil');
    await esperar(() => document.getElementById('rec-codigo') != null);
    expect(texto()).toContain('+34611222333');
    expect(texto()).not.toContain(en['recovery.phone.changed']);
    expect(document.getElementById('rec-movil')).toBeNull();
    await act(async () => {
      escribir('rec-codigo', '123456');
    });
    await enviar('movil');
    await esperar(() => texto().includes(en['recovery.phone.changed']));
    expect(pedidas.find((p) => p.clave === 'POST /api/phone')?.cuerpo).toEqual({ code: '123456' });
    expect((document.getElementById('rec-movil') as HTMLInputElement).value).toBe('+34611222333');
  });

  test('el móvil de otra cuenta se dice tras el código y se vuelve a empezar', async () => {
    await montar({ username: 'eva' });
    respuestas['PUT /api/phone'] = [202, { pending: '+34611222333' }];
    respuestas['POST /api/phone'] = [409, { error: { code: 'phone_has_account' } }];
    await act(async () => {
      escribir('rec-password', 'secreta-larga');
      escribir('rec-movil-nuevo', '+34611222333');
    });
    await enviar('movil');
    await esperar(() => document.getElementById('rec-codigo') != null);
    await act(async () => {
      escribir('rec-codigo', '654321');
    });
    await enviar('movil');
    await esperar(() => texto().includes(en['recovery.phone.has_account']));
    expect(document.getElementById('rec-codigo')).toBeNull();
    expect(document.getElementById('rec-movil-nuevo')).not.toBeNull();
  });
});

test('cada código de Sytue ID tiene su aviso; lo desconocido es «no se ha podido»', () => {
  expect(falloDeUsuario(409, 'username_taken')).toBe('taken');
  expect(falloDeUsuario(401, '')).toBe('wrong_password');
  expect(falloDeUsuario(500, 'otro')).toBe('failed');
  expect(falloDeMovil(429, 'code_wait')).toBe('wait');
  expect(falloDeMovil(429, 'sms_limit')).toBe('limit');
  expect(falloDeMovil(410, 'code_expired')).toBe('code_expired');
  expect(falloDeMovil(503, 'sms_unavailable')).toBe('unavailable');
  expect(falloDeMovil(502, '')).toBe('failed');
});
