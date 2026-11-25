/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { vaultCoreModule } from '@sytue/config/client-core';
import { PqcRuntime } from './runtime.js';

// El núcleo de la bóveda (`vault`, cmd/vault-wasm) es lo que carga un PqcRuntime sin
// módulo; `core` sigue siendo el completo. Con un Rooms anterior, `vault` arranca
// sobre `core`, que lleva las mismas operaciones (docs/decisions.md#0086).

const realWorker = globalThis.Worker;
const realLocation = (globalThis as { location?: unknown }).location;
afterEach(() => {
  globalThis.Worker = realWorker;
  (globalThis as { location?: unknown }).location = realLocation;
});

function fakeWorker(answer: (module: string) => string | null): { modules: string[] } {
  const seen = { modules: [] as string[] };
  class FakeWorker {
    onmessage: ((event: MessageEvent<unknown>) => void) | null = null;
    onerror: (() => void) | null = null;
    onmessageerror: (() => void) | null = null;
    postMessage(message: { type?: string; module?: string; id?: number }): void {
      if (message.type === 'initialize') {
        seen.modules.push(message.module!);
        const served = answer(message.module!);
        queueMicrotask(() =>
          this.onmessage?.(
            new MessageEvent('message', {
              data: served ? { type: 'ready', module_id: served } : { type: 'failed' },
            }),
          ),
        );
        return;
      }
      if (typeof message.id === 'number')
        queueMicrotask(() =>
          this.onmessage?.(
            new MessageEvent('message', {
              data: { type: 'result', id: message.id, ok: true, value: 'hecho' },
            }),
          ),
        );
    }
    terminate(): void {}
  }
  globalThis.Worker = FakeWorker as unknown as typeof Worker;
  (globalThis as { location?: unknown }).location = { origin: 'https://app.sytue.test' };
  return seen;
}

const lease = () => {
  const life = new AbortController();
  return { signal: life.signal, assertOpen: () => life.signal.throwIfAborted() };
};

test('sin módulo explícito, el runtime carga el núcleo de la bóveda', async () => {
  const seen = fakeWorker((module) => module);
  const runtime = new PqcRuntime(lease());
  expect(await runtime.invoke({ operation: 'identity.create' })).toBe('hecho');
  expect(seen.modules).toEqual([vaultCoreModule]);
  runtime.close();
  const full = new PqcRuntime(lease(), 'core');
  expect(await full.invoke({ operation: 'rooms.search.parse' })).toBe('hecho');
  expect(seen.modules).toEqual(['vault', 'core']);
  full.close();
});

test('la bóveda acepta arrancar sobre el núcleo completo, y nada más', async () => {
  fakeWorker(() => 'core');
  const vault = new PqcRuntime(lease());
  expect(await vault.invoke({ operation: 'identity.public' })).toBe('hecho');
  vault.close();
  fakeWorker(() => 'office');
  const wrong = new PqcRuntime(lease());
  await expect(wrong.invoke({ operation: 'identity.public' })).rejects.toThrow(
    'crypto_runtime_unavailable',
  );
  wrong.close();
  // Al revés no: quien pide `core` necesita lo que `vault` no lleva.
  fakeWorker(() => 'vault');
  const full = new PqcRuntime(lease(), 'core');
  await expect(full.invoke({ operation: 'rooms.search.parse' })).rejects.toThrow(
    'crypto_runtime_unavailable',
  );
  full.close();
});
