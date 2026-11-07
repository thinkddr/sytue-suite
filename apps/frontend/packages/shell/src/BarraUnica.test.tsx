/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UNA SOLA BARRA EN EL TELÉFONO, con la Sytue Shell de verdad: la topbar de la suite
 * acoge la barra de la app (AppShell, AdminShell) y la de 44 no se pinta. Y en la app
 * nativa, donde no hay topbar, la app conserva su barra.
 *
 * Visto en ROJO: con `nativo ? null : { destinos }` cambiado a `null` (no se publica el
 * contexto) salen dos barras y falla el primero.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render, type ComponentChildren } from 'preact';
import { act } from 'preact/test-utils';
import { AdminShell, AppShell } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import { SytueShell } from './SytueShell.js';

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

function barras(app: ComponentChildren): {
  arriba: number;
  header: HTMLElement;
  quitar: () => void;
} {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => {
    render(
      <SytueShell currentApp="mail" user={user} hubBaseUrl="/hub" accessToken="x" locale="es">
        {app}
      </SytueShell>,
      host,
    );
  });
  return {
    arriba: host.querySelectorAll('header[role="banner"], .sytue-mobilebar').length,
    header: host.querySelector<HTMLElement>('header[role="banner"]')!,
    quitar: () => {
      render(null, host);
      host.remove();
    },
  };
}

test('AppShell dentro de la Shell: una sola barra, con el ☰ y el título dentro', () => {
  const r = barras(
    <AppShell topbar={null} sidebar={<nav>menú</nav>} title="Recibidos">
      contenido
    </AppShell>,
  );
  expect(r.arriba).toBe(1);
  expect(r.header.querySelector('button[aria-controls]')).not.toBeNull();
  expect(r.header.textContent).toContain('Recibidos');
  // La entrada a la suite, con el avatar de la cuenta.
  expect(r.header.querySelector('button[aria-label^="Sytue:"]')).not.toBeNull();
  r.quitar();
});

test('AdminShell dentro de la Shell: una sola barra', () => {
  const r = barras(
    <AdminShell sidebar={<nav>menú</nav>} title="Mi unidad">
      contenido
    </AdminShell>,
  );
  expect(r.arriba).toBe(1);
  expect(r.header.textContent).toContain('Mi unidad');
  r.quitar();
});

test('sin barra de la app, la topbar lleva el nombre de MARCA del producto y su marca', () => {
  const r = barras(<main>contenido</main>);
  expect(r.arriba).toBe(1);
  // «Mail», el nombre de marca y sin traducir (la shell va en castellano): el mismo
  // del selector de apps y del FyneApp.toml (dueño: «mantén los nombres originales»).
  // (el «Mail» del selector de apps de escritorio sigue ahí, escondido en el teléfono).
  const titulo = r.header.querySelector('[data-marca-app] + div span')!;
  expect(titulo.textContent).toBe('Mail');
  // La marca del producto ocupa el sitio del ☰ cuando no lo hay (`peer-empty/inicio`).
  const marca = r.header.querySelector('[data-marca-app]')!;
  expect(marca.className).toContain('peer-empty/inicio:flex');
  expect(marca.previousElementSibling?.className).toContain('peer/inicio');
  expect(marca.querySelector('img')).not.toBeNull();
  r.quitar();
});
