/* SPDX-License-Identifier: Apache-2.0 */
/**
 * EL «NO» DEL NÚCLEO DICE QUÉ PUERTO DEL ANFITRIÓN FALLÓ.
 *
 * Cuando un puerto del anfitrión falla (una petición al servicio, la caché…), el
 * núcleo solo ve el rechazo y contesta `crypto_rejected`. Mail se quedaba así en
 * `prepare/error.Error` sin forma de saber qué había pasado de verdad. El rechazo lleva
 * ahora `hostFailure`: el método del puerto y su error.
 *
 * Visto en ROJO sin `ultimoFalloAnfitrion` en el runtime: el rechazo no lleva nada.
 */
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { PqcRuntime } from './runtime.js';

const binding = {
  scope: { tenant: 'acme', subject: 'ana', product: 'mail' as const },
  matrixUserID: '@ana:matrix.test',
  deviceID: 'PQDEVICE',
};

class FakeWorker {
  static all: FakeWorker[] = [];
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onmessageerror: (() => void) | null = null;
  // Lo que hace el núcleo con cada petición: aquí, llamar al anfitrión y decir «no».
  alPedir: ((id: number) => void) | null = null;
  constructor() {
    FakeWorker.all.push(this);
  }
  postMessage(value: unknown): void {
    const message = value as { id?: number; type?: string };
    if (typeof message.id === 'number' && message.type !== 'host.result') {
      const id = message.id;
      queueMicrotask(() => this.alPedir?.(id));
    }
  }
  terminate(): void {}
  emit(data: unknown): void {
    this.onmessage?.({ data } as MessageEvent);
  }
}

const original = {
  Worker: (globalThis as { Worker?: unknown }).Worker,
  location: (globalThis as { location?: unknown }).location,
};
beforeEach(() => {
  FakeWorker.all = [];
  Object.assign(globalThis, {
    Worker: FakeWorker,
    location: { origin: 'https://mail.sytue.test' },
  });
});
afterEach(() => {
  Object.assign(globalThis, original);
});

test('un crypto_rejected tras fallar un puerto lleva el método y el error del puerto', async () => {
  const vida = new AbortController();
  const lease = { binding, signal: vida.signal, assertOpen: () => vida.signal.throwIfAborted() };
  const runtime = new PqcRuntime(lease, 'mail');
  const fallo = Object.assign(new Error('servicio'), { name: 'HttpError', status: 409 });
  const puerto = runtime.registerHostPort({
    scope: binding.scope,
    lease,
    run: async (action, signal) => action(signal),
    handlers: {
      'source.capabilities': async () => {
        throw fallo;
      },
    },
    cleanup: async () => {},
  });
  const pendiente = runtime.invoke({ operation: 'mail.capabilities', port: puerto.id });
  await Promise.resolve();
  const w = FakeWorker.all[0]!;
  w.alPedir = (id) => {
    // El núcleo pide al anfitrión y, cuando este contesta que no, rechaza la petición.
    w.emit({
      type: 'host.call',
      id: 1,
      port: puerto.id,
      scope: binding.scope,
      method: 'source.capabilities',
      payload: new Uint8Array(4),
    });
    setTimeout(() => w.emit({ type: 'result', id, ok: false }), 20);
  };
  w.emit({ type: 'ready', module_id: 'mail' });
  const error = (await pendiente.catch((e: unknown) => e)) as Error & {
    hostFailure?: { method: string; error: unknown };
  };
  expect(error).toBeInstanceOf(Error);
  expect(error.message).toBe('crypto_rejected');
  expect(error.hostFailure?.method).toBe('source.capabilities');
  expect(error.hostFailure?.error).toBe(fallo);
  vida.abort();
});

test('un crypto_rejected lleva el motivo fijo que dio el núcleo', async () => {
  const vida = new AbortController();
  const lease = { binding, signal: vida.signal, assertOpen: () => vida.signal.throwIfAborted() };
  const runtime = new PqcRuntime(lease, 'mail');
  const pendiente = runtime.invoke({ operation: 'mail.open' });
  await Promise.resolve();
  const w = FakeWorker.all[0]!;
  w.alPedir = (id) =>
    w.emit({
      type: 'result',
      id,
      ok: false,
      code: 'crypto_rejected',
      motivo: 'abrir.preparar.entrada_no_lista.no_listo',
    });
  w.emit({ type: 'ready', module_id: 'mail' });
  const error = (await pendiente.catch((e: unknown) => e)) as Error & { coreReason?: unknown };
  expect(error.message).toBe('crypto_rejected');
  expect(error.coreReason).toBe('abrir.preparar.entrada_no_lista.no_listo');
  vida.abort();
});
