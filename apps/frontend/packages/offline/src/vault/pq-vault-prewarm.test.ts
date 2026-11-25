/* SPDX-License-Identifier: Apache-2.0 */
/**
 * LA APERTURA DE LA BÓVEDA ARRANCA EL NÚCLEO (`PqVault.open`), no solo la puerta: el
 * worker del núcleo `vault` nace antes de pedir el bootstrap, también para quien abre
 * la bóveda sin puerta (Photos y Docs con el Drive de la bóveda). Nace sin cuenta (solo
 * la URL pública de los artefactos) y, si la apertura acaba sin adoptarlo, se termina.
 *
 * Visto en ROJO: sin el `prewarmClientCore` de `openInternal`, ningún worker antes del
 * bootstrap.
 */
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { PqVault, PqVaultError } from './pq-vault.js';

class FakeWorker {
  static all: FakeWorker[] = [];
  posted: unknown[] = [];
  terminated = false;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onmessageerror: (() => void) | null = null;
  constructor(
    readonly url: URL,
    readonly options: WorkerOptions,
  ) {
    FakeWorker.all.push(this);
  }
  postMessage(value: unknown): void {
    this.posted.push(value);
  }
  terminate(): void {
    this.terminated = true;
  }
}

const original = {
  Worker: (globalThis as { Worker?: unknown }).Worker,
  location: (globalThis as { location?: unknown }).location,
};
beforeEach(() => {
  FakeWorker.all = [];
  Object.assign(globalThis, { Worker: FakeWorker, location: { origin: 'https://photos.test' } });
});
afterEach(() => {
  Object.assign(globalThis, original);
});

const actor = '11111111-1111-4111-8111-111111111111';

test('PqVault.open starts the vault core before the bootstrap and drops it if unused', async () => {
  const antesDelBootstrap: number[] = [];
  const fetchImpl = (async () => {
    antesDelBootstrap.push(FakeWorker.all.length);
    return new Response('{}', { status: 503 });
  }) as unknown as typeof fetch;
  await expect(
    PqVault.open({ actor, tenant: 'acme', product: 'photos', basePath: '/photos', fetchImpl }),
  ).rejects.toBeInstanceOf(PqVaultError);
  expect(antesDelBootstrap[0]).toBe(1);
  const [worker] = FakeWorker.all;
  // Sin cuenta: solo la URL pública de los artefactos del núcleo ligero.
  expect(worker!.posted).toEqual([
    { type: 'initialize', assetBaseURL: 'https://photos.test/api/client-core/', module: 'vault' },
  ]);
  // Nadie lo adoptó: la apertura lo suelta al acabar.
  expect(worker!.terminated).toBe(true);
});

test('with its own core (`createCore`) the opening starts nothing', async () => {
  const fetchImpl = (async () => new Response('{}', { status: 503 })) as unknown as typeof fetch;
  await expect(
    PqVault.open({
      actor,
      tenant: 'acme',
      product: 'photos',
      fetchImpl,
      createCore: () => {
        throw new Error('not reached');
      },
    }),
  ).rejects.toBeInstanceOf(PqVaultError);
  expect(FakeWorker.all).toHaveLength(0);
});
