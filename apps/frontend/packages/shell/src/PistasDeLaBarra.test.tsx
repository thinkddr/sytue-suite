/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * TODO BOTÓN DE LA BARRA DICE QUÉ ES CON EL TECLADO, con la Sytue Shell de verdad: al
 * enfocarlo sale su pista, y la pista dice su nombre accesible.
 *
 * La campana y el candado ya la llevaban (`TopBarButton`); el avatar de la cuenta, la
 * marca que abre el selector de aplicaciones y el selector compacto del teléfono, no:
 * eran botones escritos a mano —el compacto con un `title`, que no sale ni con el
 * teclado ni con el dedo—. En Fyne el avatar tampoco la tenía
 * (`TestLaCuentaDeLaBarraDiceDeQuienEsYLoEnseñaEnSuPista`).
 *
 * VISTO EN ROJO con el disparador de AccountMenu.tsx devuelto a su `<button>` de antes:
 * «Cuenta de Ana García» sin pista.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
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

const pista = (): string | null =>
  document.querySelector('[role="tooltip"]')?.firstChild?.textContent ?? null;

async function hasta(cond: () => boolean, que: string, limite = 2000): Promise<void> {
  const fin = Date.now() + limite;
  while (!cond() && Date.now() < fin) await new Promise((r) => setTimeout(r, 10));
  if (!cond()) throw new Error(`no se cumplió en ${limite} ms: ${que}`);
}

test('cada botón de la barra enseña su pista al enfocarlo, con su nombre', async () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => {
    render(
      <SytueShell currentApp="mail" user={user} hubBaseUrl="/hub" accessToken="x" locale="es">
        <main>contenido</main>
      </SytueShell>,
      host,
    );
  });
  const header = host.querySelector<HTMLElement>('header[role="banner"]')!;
  const botones = [...header.querySelectorAll<HTMLButtonElement>('button')];
  const nombres = botones.map((b) => b.getAttribute('aria-label'));
  // Los que esta prueba existe para guardar, que estén.
  expect(nombres).toContain('Cuenta de Ana García');
  expect(nombres).toContain('Sytue One — aplicaciones de la suite');
  expect(nombres).toContain('Mail · Sytue One — aplicaciones de la suite');
  const sinPista: string[] = [];
  for (const b of botones) {
    const nombre = b.getAttribute('aria-label') ?? '';
    // Ningún botón de la barra se explica con `title`: no sale con el teclado ni con el dedo.
    expect(b.hasAttribute('title')).toBe(false);
    act(() => b.focus());
    try {
      await hasta(() => pista() === nombre, `la pista de «${nombre}»`, 500);
    } catch {
      sinPista.push(nombre);
    }
    act(() => b.blur());
    await hasta(() => pista() === null, 'perder el foco la cierra');
  }
  expect(sinPista).toEqual([]);
  render(null, host);
  host.remove();
});
