/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * MI CUENTA SE RECORRE POR SU ÍNDICE Y SE ENLAZA POR SECCIÓN.
 *
 * Eran ocho tarjetas en dos columnas, sin título, sin descripción y sin forma de
 * enlazar a «Seguridad»; `?mfa=required` ni siquiera llevaba allí. Se vigila, con la
 * página entera montada:
 *
 * - que hay título (h1) y un índice con una entrada por tarjeta, y que cada entrada
 *   apunta a una tarjeta que existe con ese `id`;
 * - que las anclas que ya circulan fuera —las de la revisión de seguridad
 *   (`ANCLAS` de `@sytue/shell`, que también usan el portal y el Hub) y las viejas
 *   `#app-passwords` (Sytue Mail) y `#claves` (Sytue ID)— llevan a una sección;
 * - que `?mfa=required` abre en Seguridad con el foco en su título;
 * - que el plan solo sale, en la página y en el índice, en cuentas personales.
 *
 * Vistos en ROJO con la página de antes (sin índice ni `h1`) y quitando
 * `seccionInicial` (falla «?mfa=required abre en Seguridad»).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { SytueUser } from '@sytue/auth';
import { ANCLAS } from '@sytue/shell';
import { seccionDelFragmento } from '@sytue/ui';
import { useT as traductor } from '../i18n/index.js';
import { en } from '../i18n/messages/en.js';
import { olvidarLecturas } from '../lib/primeraLectura.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.test/account/' });
const fetchOriginal = globalThis.fetch;
const scrollOriginal = HTMLElement.prototype.scrollIntoView;

const { AccountApp, indiceDeCuenta } = await import('./AccountApp.js');

const t = traductor('en');

const USUARIO: SytueUser = {
  id: 'u1',
  email: 'ana@example.eu',
  emailVerified: true,
  name: 'Ana',
  preferredUsername: 'ana',
  roles: [],
  groups: [],
  tenant: 'acme',
};

let desplazados: string[] = [];

beforeEach(() => {
  desplazados = [];
  HTMLElement.prototype.scrollIntoView = function (this: HTMLElement) {
    desplazados.push(this.id);
  };
  // El token, para que la Sytue Shell no mande al login (navegar borraría el `#id`);
  // lo demás, 404: las tarjetas se pintan igual, con su aviso de fallo.
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const token = String(input).endsWith('/auth/token');
    return new Response(
      JSON.stringify(
        token ? { accessToken: 'tk', expiresAt: Date.now() + 3_600_000 } : { error: 'not_found' },
      ),
      { status: token ? 200 : 404, headers: { 'content-type': 'application/json' } },
    );
  }) as unknown as typeof fetch;
});

afterEach(() => {
  act(() => render(null, document.body));
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
  HTMLElement.prototype.scrollIntoView = scrollOriginal;
  window.history.replaceState(null, '', '/account/');
  olvidarLecturas();
});

afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

async function montar(user: SytueUser, mfaRequired = false): Promise<void> {
  await act(async () => {
    render(
      <AccountApp
        user={user}
        hubBaseUrl="https://hub.sytue.test"
        idBaseUrl="https://id.sytue.test"
        locale="en"
        nombreInicial="Ana"
        email="ana@example.eu"
        mfaRequired={mfaRequired}
      />,
      document.body,
    );
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 40));
  });
}

const entradas = (): string[] =>
  [...document.querySelectorAll('main nav a')].map((a) => a.getAttribute('href') ?? '');

describe('Mi cuenta con índice', () => {
  test('tiene título y un índice cuyas entradas llevan a tarjetas que existen', async () => {
    await montar(USUARIO);
    expect(document.querySelector('main h1')?.textContent).toBe(en['app.title']);
    expect(document.querySelector('main')?.textContent).toContain(en['app.lead']);
    const hrefs = entradas();
    expect(hrefs).toEqual([
      '#perfil',
      '#seguridad',
      '#recuperacion',
      '#sesiones',
      '#dispositivos',
      '#actividad',
      '#contrasenas-app',
      '#firma',
      '#borrar',
    ]);
    for (const href of hrefs) {
      const tarjeta = document.getElementById(href.slice(1));
      expect(tarjeta, href).not.toBeNull();
      // El título de la tarjeta es lo que recibe el foco al saltar.
      expect(tarjeta!.querySelector('h2'), href).not.toBeNull();
    }
  });

  test('las anclas que ya circulan fuera llevan a una sección del índice', () => {
    const indice = indiceDeCuenta(t, true);
    for (const ancla of Object.values(ANCLAS)) {
      expect(seccionDelFragmento(indice, ancla), ancla).toBe(ancla.slice(1));
    }
    expect(seccionDelFragmento(indice, '#app-passwords')).toBe('contrasenas-app');
    expect(seccionDelFragmento(indice, '#claves')).toBe('seguridad');
  });

  test('?mfa=required abre en Seguridad, con el foco en su título', async () => {
    window.history.replaceState(null, '', '/account/?mfa=required');
    await montar(USUARIO, true);
    expect(desplazados).toEqual(['seguridad']);
    expect(document.activeElement).toBe(document.querySelector('#seguridad h2'));
    expect(
      document.querySelector('main nav a[aria-current="location"]')?.getAttribute('href'),
    ).toBe('#seguridad');
  });

  test('/account#sesiones abre en Sesiones', async () => {
    window.history.replaceState(null, '', '/account/#sesiones');
    await montar(USUARIO);
    expect(desplazados).toEqual(['sesiones']);
    expect(document.activeElement).toBe(document.querySelector('#sesiones h2'));
  });

  test('el plan solo en cuentas personales, en la página y en el índice', async () => {
    await montar(USUARIO);
    expect(entradas()).not.toContain('#plan');
    expect(document.getElementById('plan')).toBeNull();
    act(() => render(null, document.body));

    await montar({ ...USUARIO, orgKind: 'personal' });
    expect(entradas()).toContain('#plan');
    expect(document.getElementById('plan')).not.toBeNull();
  });
});
