/**
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Que `logout()` navegue al prefijo de SU app y no a la raíz del host.
 *
 * Medido contra producción el 2026-09-07: en `corporate.sytue.eu`
 * —host de organización, donde las diez superficies viven con prefijo—
 * `/auth/logout` devuelve 404 y `/one/auth/logout` devuelve 302. Cerrar sesión
 * no cerraba nada y no lo decía.
 */
import { describe, expect, it, afterEach } from 'bun:test';

type VentanaFalsa = {
  __sytueBase?: string;
  location: { assign: (url: string) => void };
};

const original = (globalThis as { window?: unknown }).window;
afterEach(() => {
  (globalThis as { window?: unknown }).window = original;
});

/** Monta un `window` con el base publicado y captura a dónde se navega. */
async function destinoDelLogout(base: string | undefined): Promise<string> {
  let destino = '';
  const w: VentanaFalsa = { location: { assign: (u) => (destino = u) } };
  if (base !== undefined) w.__sytueBase = base;
  (globalThis as { window?: unknown }).window = w;
  const { logout } = await import('./index.js');
  logout();
  return destino;
}

describe('logout bajo el host de organización', () => {
  it('lleva el prefijo de la app cuando el servidor lo publica', async () => {
    expect(await destinoDelLogout('/one')).toBe('/one/auth/logout');
  });

  it('no lo lleva cuando la app se sirve en la raíz', async () => {
    // Publicado y VACÍO es un valor, no una ausencia: es el caso de una app en
    // su propio subdominio, donde `/one/...` sería el 404.
    expect(await destinoDelLogout('')).toBe('/auth/logout');
  });

  it('sin base publicado se comporta como en la raíz', async () => {
    expect(await destinoDelLogout(undefined)).toBe('/auth/logout');
  });

  it('no duplica el prefijo si ya viniera puesto', async () => {
    // Defensa de la idempotencia: `/one/one/auth/logout` sería otro 404.
    expect(await destinoDelLogout('/one')).not.toContain('/one/one');
  });
});

it('waits for secret cleanup before logout even when a second cleanup rejects', async () => {
  const events: string[] = [];
  (globalThis as { window?: unknown }).window = {
    __sytueBase: '/rooms',
    location: { assign: (url: string) => events.push(url) },
  };
  const { logout, registerLogoutCleanup } = await import('./index.js');
  let finish: () => void = () => undefined;
  const unregister = registerLogoutCleanup(
    () =>
      new Promise<void>((resolve) => {
        finish = () => {
          events.push('keys removed');
          resolve();
        };
      }),
  );
  const unregisterFailed = registerLogoutCleanup(async () => {
    throw new Error('already closed');
  });
  try {
    logout();
    expect(events).toEqual([]);
    finish();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(events).toEqual(['keys removed', '/rooms/auth/logout']);
  } finally {
    unregister();
    unregisterFailed();
  }
});
