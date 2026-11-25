/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL BUSCADOR DE LA BARRA SE ANUNCIA AL LECTOR DE PANTALLA. El lector solo sigue
 * `aria-activedescendant` en el elemento ENFOCADO, y el combobox estaba en la caja que
 * envuelve el campo, que nunca tiene el foco: al bajar con las flechas el resultado se
 * marcaba en pantalla y no se anunciaba, ni si la lista estaba abierta. Ahora es el
 * patrón de WAI-ARIA 1.2, el mismo de `PaletaDeComandos`: el campo es el combobox.
 *
 * VISTO EN ROJO con GlobalSearch.tsx de antes: el campo enfocado no tiene `role`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { HubClient, SearchResponse } from '@sytue/api-clients';
import type { SytueUser } from '@sytue/auth';
import { I18nProvider } from '@sytue/i18n/preact';
import { ShellContext } from './context.js';
import { GlobalSearch } from './GlobalSearch.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const user: SytueUser = {
  id: 'u1',
  email: 'ana@sytue.test',
  emailVerified: true,
  name: 'Ana García',
  preferredUsername: 'ana',
  roles: [],
  groups: [],
};

const hub = {
  search: async (): Promise<SearchResponse> =>
    ({
      results: [
        { id: '1', source: 'drive', title: 'Informe anual', url: '/drive/1', snippet: 'PDF' },
        { id: '2', source: 'drive', title: 'Informe mensual', url: '/drive/2' },
        { id: '3', source: 'mail', title: 'Re: informe', url: '/mail/3' },
      ],
      fallidos: [],
    }) as unknown as SearchResponse,
} as unknown as HubClient;

async function hasta(cond: () => boolean, que: string, limite = 3000): Promise<void> {
  const fin = Date.now() + limite;
  while (!cond() && Date.now() < fin) await new Promise((r) => setTimeout(r, 10));
  if (!cond()) throw new Error(`no se cumplió en ${limite} ms: ${que}`);
}

test('el campo enfocado es el combobox: expandido, con su lista y el resultado marcado', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  act(() => {
    render(
      <I18nProvider locale="es">
        <ShellContext.Provider
          value={{ currentApp: 'drive', hub, hubBaseUrl: '/hub', user, accessToken: 'x' }}
        >
          <GlobalSearch />
        </ShellContext.Provider>
      </I18nProvider>,
      host,
    );
  });
  const campo = host.querySelector<HTMLInputElement>('input[type="search"]')!;
  act(() => campo.focus());
  // El nombre y el papel van en lo que tiene el foco, que es lo que el lector lee.
  expect(document.activeElement).toBe(campo);
  expect(campo.getAttribute('role')).toBe('combobox');
  expect(campo.getAttribute('aria-label')).toBe('Búsqueda global');
  expect(campo.getAttribute('aria-expanded')).toBe('false');
  // Y no hay un segundo combobox alrededor que compita con él.
  expect(host.querySelectorAll('[role="combobox"]')).toHaveLength(1);

  act(() => {
    campo.value = 'informe';
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await hasta(() => host.querySelectorAll('[role="option"]').length === 3, 'llegan los resultados');
  expect(campo.getAttribute('aria-expanded')).toBe('true');
  const lista = document.getElementById(campo.getAttribute('aria-controls') ?? '');
  expect(lista?.querySelector('[role="listbox"]')).not.toBeNull();

  // Las flechas mueven el marcado y el CAMPO dice cuál es: el lector lo anuncia.
  const marcado = () =>
    document.getElementById(campo.getAttribute('aria-activedescendant') ?? '')?.textContent;
  expect(marcado()).toContain('Informe anual');
  act(() => {
    campo.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
  });
  expect(marcado()).toContain('Informe mensual');
  expect(
    document
      .getElementById(campo.getAttribute('aria-activedescendant')!)
      ?.getAttribute('aria-selected'),
  ).toBe('true');
  act(() => render(null, host));
  host.remove();
});
