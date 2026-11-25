/* SPDX-License-Identifier: Apache-2.0 */
/**
 * LA PUERTA NO DEJA LA APP EN BLANCO (ADR 0086, roadmap «Rendimiento y estética de la
 * web», 2026-11-24): Calendar tardaba 4,5 s en escritorio y ~21 s en el móvil en ser
 * usable porque la puerta no pintaba nada hasta abrir la bóveda, y la mayoría de las
 * apps no guardan nada en ella.
 *
 * Lo que se fija aquí, y que se vio en ROJO invirtiendo cada guardián a mano:
 * - en segundo plano la app se pinta antes de abrir la bóveda y no se vuelve a montar;
 * - con hijos que reciben la bóveda, el esqueleto no recibe nada de ella y los hijos
 *   no se llaman hasta que la bóveda abre Y su identidad cuadra;
 * - el núcleo se calienta a la vez que el bootstrap, y se suelta si nadie lo usa;
 * - el candado va a la barra de la suite y ya no flota sobre la app.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { MARCA_APP_LISTA, marcarAppLista } from '@sytue/ui';
import { PqVaultError, pqVaultProfile, type PqVault } from '@sytue/offline/vault';
import type { SytueUser } from '@sytue/auth';
import { PqVaultGate } from './PqVaultGate.js';
import { SytueShell } from '../SytueShell.js';
import {
  PERFIL_CIFRADO,
  estadoPuertaCifrado,
} from '../../../../../../test/e2e/tests/puerta-cifrado.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const actor = '11111111-1111-4111-8111-111111111111';
const keyId = 'abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789';

async function until(check: () => boolean): Promise<void> {
  for (let i = 0; i < 150; i++) {
    if (check()) return;
    await act(async () => {
      await new Promise((r) => setTimeout(r, 10));
    });
  }
  throw new Error('timeout waiting for condition');
}

function fakeVault(product: string, subject = actor): PqVault {
  let closed = false;
  return {
    scope: Object.freeze({ tenant: 'acme', subject, product }),
    profile: pqVaultProfile,
    idOrigin: 'https://id.sytue.eu',
    vaultId: '!pq_testvault',
    keyId,
    recipient: 'age1pq1testrecipient',
    get isClosed() {
      return closed;
    },
    close() {
      closed = true;
    },
    async closeAndDrain() {
      closed = true;
    },
  } as unknown as PqVault;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const user: SytueUser = {
  id: actor,
  tenant: 'acme',
  email: 'ana@sytue.test',
  emailVerified: true,
  name: 'Ana García',
  preferredUsername: 'ana',
  roles: [],
  groups: [],
};

const roots: HTMLElement[] = [];
function root(): HTMLElement {
  const el = document.createElement('div');
  document.body.appendChild(el);
  roots.push(el);
  return el;
}
afterEach(async () => {
  for (const el of roots.splice(0)) {
    await act(() => {
      render(null, el);
    });
    el.remove();
  }
  document.body.innerHTML = '';
  performance.clearMarks(MARCA_APP_LISTA);
});

const botones = (): HTMLButtonElement[] => [...document.querySelectorAll('button')];

test('en segundo plano la app se pinta ya, la bóveda espera a que esté lista y no la vuelve a montar', async () => {
  performance.clearMarks(MARCA_APP_LISTA);
  const pasos: string[] = [];
  const abrir = deferred<PqVault>();
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="calendar"
        background
        prewarm={() => {
          pasos.push('núcleo');
          return () => pasos.push('soltar');
        }}
        openVault={() => {
          pasos.push('abrir');
          return abrir.promise;
        }}
      >
        <p data-app="true">Llamada con proveedor</p>
      </PqVaultGate>,
      el,
    );
  });
  // Pintada antes de abrir nada: ni el núcleo ni la bóveda se han pedido aún.
  const app = el.querySelector('[data-app]');
  expect(app?.textContent).toBe('Llamada con proveedor');
  expect(pasos).toEqual([]);
  expect(estadoPuertaCifrado(PERFIL_CIFRADO)).toBe('abierta');
  expect(el.textContent).not.toContain(pqVaultProfile);

  // La app ya enseña sus datos: ahora sí, núcleo y bóveda, en ese orden.
  marcarAppLista();
  await until(() => pasos.includes('abrir'));
  expect(pasos).toEqual(['núcleo', 'abrir']);
  abrir.resolve(fakeVault('calendar'));
  await until(() => pasos.includes('soltar'));
  // La misma app, el mismo nodo: abrir la bóveda no la volvió a montar.
  expect(el.querySelector('[data-app]')).toBe(app);
});

test('en segundo plano un fallo de la bóveda no tapa la app: queda en el candado, con sus salidas', async () => {
  marcarAppLista();
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="notes"
        background
        // El viaje automático a Sytue ID ya se hizo (o no se puede): queda el candado.
        autoSignIn={false}
        openVault={async () => {
          throw new PqVaultError('signin');
        }}
      >
        <p data-app="true">Notas</p>
      </PqVaultGate>,
      el,
    );
  });
  const candado = (): HTMLButtonElement | undefined =>
    botones().find((b) => b.getAttribute('aria-label') === 'Tus datos cifrados necesitan atención');
  await until(() => candado() !== undefined);
  expect(el.querySelector('[data-app]')).not.toBeNull();
  expect(estadoPuertaCifrado(PERFIL_CIFRADO)).toBe('abierta');
  await act(() => {
    candado()!.click();
  });
  await until(() => document.querySelector('[role="dialog"]') !== null);
  const dialogo = document.querySelector('[role="dialog"]')!;
  expect(
    [...dialogo.querySelectorAll('button')].some(
      (b) => b.textContent?.trim() === 'Iniciar sesión en Sytue ID',
    ),
  ).toBe(true);
});

test('con hijos que reciben la bóveda: el esqueleto mientras abre y nada descifrado antes de su identidad', async () => {
  const llamadas: PqVault[] = [];
  const abrir = deferred<PqVault>();
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="drive"
        openVault={() => abrir.promise}
        skeleton={<div data-esqueleto="true" />}
      >
        {(vault) => {
          llamadas.push(vault);
          return <p data-secreto="true">{vault.keyId}</p>;
        }}
      </PqVaultGate>,
      el,
    );
  });
  // El armazón, la franja discreta, y nada de la bóveda.
  expect(el.querySelector('[data-esqueleto]')).not.toBeNull();
  expect(el.querySelector('[role="status"]')?.textContent).toContain('Desbloqueando');
  expect(el.querySelector('h1')).toBeNull();
  expect(estadoPuertaCifrado(PERFIL_CIFRADO)).toBe('abriendo');
  expect(llamadas).toHaveLength(0);

  // Abre una bóveda de OTRA persona: no se llama a los hijos, ni con esqueleto.
  abrir.resolve(fakeVault('drive', '22222222-2222-4222-8222-222222222222'));
  await until(() => el.querySelector('[data-sytue-boveda="cerrada"]') !== null);
  expect(llamadas).toHaveLength(0);
  expect(el.querySelector('[data-esqueleto]')).toBeNull();
  expect(el.querySelector('[data-secreto]')).toBeNull();
  expect(el.innerHTML).not.toContain(keyId.slice(0, 16));
});

test('con hijos que reciben la bóveda, la abierta y suya llega a los hijos', async () => {
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="drive"
        openVault={async () => fakeVault('drive')}
        skeleton={<div data-esqueleto="true" />}
      >
        {(vault) => <p data-secreto="true">{vault.vaultId}</p>}
      </PqVaultGate>,
      el,
    );
  });
  await until(() => el.querySelector('[data-secreto]') !== null);
  expect(el.querySelector('[data-esqueleto]')).toBeNull();
  expect(estadoPuertaCifrado(PERFIL_CIFRADO)).toBe('abierta');
});

test('el núcleo se calienta a la vez que el bootstrap y se suelta si la bóveda no lo usa', async () => {
  const pasos: string[] = [];
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="mail"
        prewarm={() => {
          pasos.push('núcleo');
          return () => pasos.push('soltar');
        }}
        openVault={async () => {
          // El bootstrap: el núcleo ya está bajando.
          pasos.push('bootstrap');
          throw new PqVaultError('signin');
        }}
      >
        {() => <p>correo</p>}
      </PqVaultGate>,
      el,
    );
  });
  await until(() => pasos.includes('soltar'));
  expect(pasos).toEqual(['núcleo', 'bootstrap', 'soltar']);
});

test('el candado va a la barra de la suite y no flota sobre la app (puerta por fuera de la shell)', async () => {
  marcarAppLista();
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="contacts"
        background
        openVault={async () => fakeVault('contacts')}
      >
        <SytueShell currentApp="contacts" user={user} hubBaseUrl="/hub" accessToken="x">
          <button type="button">Nuevo contacto</button>
        </SytueShell>
      </PqVaultGate>,
      el,
    );
  });
  const enBarra = (): Element | null =>
    el.querySelector('header [data-sytue-boveda-estado="abierta"]');
  await until(() => enBarra() !== null);
  // Ni un botón fijo fuera de la barra: «Nuevo contacto» queda a la vista.
  expect(el.querySelectorAll('button.fixed')).toHaveLength(0);
  await act(() => {
    (enBarra() as HTMLButtonElement).click();
  });
  await until(() => document.querySelector('[role="dialog"]') !== null);
  const dialogo = document.querySelector('[role="dialog"]')!;
  expect(dialogo.textContent).toContain(pqVaultProfile);
  // El perfil, dentro de «Más información».
  expect(dialogo.querySelector('details')?.textContent).toContain(pqVaultProfile);
});

test('el candado va a la barra de la suite también con la puerta por dentro (Drive, Docs)', async () => {
  const abrir = deferred<PqVault>();
  const el = root();
  await act(() => {
    render(
      <SytueShell currentApp="drive" user={user} hubBaseUrl="/hub" accessToken="x">
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="es"
          product="drive"
          openVault={() => abrir.promise}
          skeleton={<div data-esqueleto="true" />}
        >
          {() => <p data-app="true">Mi unidad</p>}
        </PqVaultGate>
      </SytueShell>,
      el,
    );
  });
  await until(() => el.querySelector('header [data-sytue-boveda-estado="abriendo"]') !== null);
  abrir.resolve(fakeVault('drive'));
  await until(() => el.querySelector('header [data-sytue-boveda-estado="abierta"]') !== null);
  expect(el.querySelector('[data-app]')).not.toBeNull();
  expect(el.querySelectorAll('button.fixed')).toHaveLength(0);
});

test('sin barra (shell nativa), el candado de reserva se aparta del botón flotante de la app', async () => {
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="docs"
        openVault={async () => fakeVault('docs')}
      >
        <p data-app="true">Recientes</p>
      </PqVaultGate>,
      el,
    );
  });
  await until(() => el.querySelector('[data-app]') !== null);
  const candado = el.querySelector<HTMLButtonElement>('button.fixed');
  expect(candado).not.toBeNull();
  expect(candado!.className).toContain('[body:has([data-sytue-fab])_&]:bottom-');
});

test('la pantalla de la puerta: sin jerga a la vista y «Vaciar» aparte de «Reintentar»', async () => {
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="notes"
        autoSignIn={false}
        openVault={async () => {
          throw new PqVaultError('wrong', undefined, { vault_id: '!pqv', version: 1 });
        }}
      >
        <p data-app="true">datos</p>
      </PqVaultGate>,
      el,
    );
  });
  const boton = (texto: string): HTMLButtonElement | undefined =>
    botones().find((b) => b.textContent?.trim() === texto);
  await until(() => boton('Vaciar los datos cifrados') !== undefined);
  const pantalla = el.querySelector('[data-sytue-boveda="cerrada"]')!;
  // El algoritmo y el perfil, solo dentro de «Más información».
  const detalles = pantalla.querySelector('details')!;
  expect(detalles.querySelector('summary')?.textContent).toBe('Más información');
  expect(detalles.textContent).toContain('ML-KEM-768');
  for (const p of pantalla.querySelectorAll('p'))
    if (!p.closest('details')) expect(p.textContent).not.toContain('ML-KEM-768');
  // Lo destructivo no se ve como «Reintentar» ni va en su misma fila.
  const vaciar = boton('Vaciar los datos cifrados')!;
  const reintentar = boton('Reintentar')!;
  expect(vaciar.className).not.toBe(reintentar.className);
  expect(vaciar.parentElement).not.toBe(reintentar.parentElement);
  expect(vaciar.parentElement?.className).toContain('border-t');
});

test('en segundo plano, sin la clave aquí, la puerta también va sola a Sytue ID (sin pulsar el candado)', async () => {
  sessionStorage.clear();
  marcarAppLista();
  const pasos: string[] = [];
  const el = root();
  await act(() => {
    render(
      <PqVaultGate
        actor={actor}
        tenant="acme"
        locale="es"
        product="calendar"
        basePath="/calendar"
        background
        openVault={async () => {
          throw new PqVaultError('signin');
        }}
        navigate={(url) => pasos.push(url)}
      >
        <p data-app="true">Llamada con proveedor</p>
      </PqVaultGate>,
      el,
    );
  });
  await until(() => pasos.length > 0);
  const url = new URL(pasos[0]!, 'https://calendar.test');
  expect(url.pathname).toBe('/calendar/auth/login');
  expect(url.searchParams.get('unlock')).toBe('1');
  // La app sigue a la vista mientras tanto.
  expect(el.querySelector('[data-app]')).not.toBeNull();
});
