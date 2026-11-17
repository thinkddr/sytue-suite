import { describe, expect, test } from 'bun:test';
import { currentConnectivity } from './native.js';
import { clearOfflineStorage } from './storage.js';

async function within<T>(promise: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      promise,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(new Error('test deadline')), 3000);
      }),
    ]);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}

describe('currentConnectivity', () => {
  // Este test corre en Bun/Node, o sea EXACTAMENTE el entorno del SSR: hay
  // `navigator`, pero no tiene `onLine`. Con `!navigator.onLine` daba 'offline'
  // y el servidor mandaba a todo el mundo el HTML de «sin conexión».
  test('en SSR (navigator sin onLine) no inventa una desconexión', () => {
    expect(navigator.onLine).toBeUndefined();
    expect(currentConnectivity()).toBe('online');
  });

  test('en el navegador respeta onLine', () => {
    const original = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
    try {
      for (const [onLine, esperado] of [
        [false, 'offline'],
        [true, 'online'],
      ] as const) {
        Object.defineProperty(globalThis, 'navigator', {
          value: { onLine },
          configurable: true,
        });
        expect(currentConnectivity()).toBe(esperado);
      }
    } finally {
      if (original) Object.defineProperty(globalThis, 'navigator', original);
    }
  });
});

describe('native scope hand-off', () => {
  test('a disposed mount cannot initialise a worker after its registration finishes', async () => {
    const originals = new Map(
      ['window', 'navigator', 'document', 'location', 'performance'].map((name) => [
        name,
        Object.getOwnPropertyDescriptor(globalThis, name),
      ]),
    );
    const messages: unknown[] = [];
    let ready!: (value: unknown) => void;
    const worker = { postMessage: (value: unknown) => messages.push(value) };
    const serviceWorker = {
      register: async () => ({ active: worker }),
      ready: new Promise((resolve) => {
        ready = resolve;
      }),
      controller: worker,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    };
    const replace = (name: string, value: unknown): void => {
      Object.defineProperty(globalThis, name, { value, configurable: true });
    };
    try {
      replace('window', {
        Capacitor: {},
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      });
      replace('navigator', { serviceWorker });
      replace('document', { querySelectorAll: () => [] });
      replace('location', { href: 'https://suite.invalid/mail' });
      replace('performance', { getEntriesByType: () => [] });
      const { activateNativeOffline } = await import('./native.js');
      const stop = activateNativeOffline({
        ownerId: 'ada',
        tenantId: 'acme',
        product: 'mail',
        scopeId: 'old-mount',
      });
      stop();
      ready({ active: worker });
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      expect(messages).toEqual([{ type: 'INVALIDATE', scopeId: 'old-mount' }]);
    } finally {
      for (const [name, descriptor] of originals) {
        if (descriptor) Object.defineProperty(globalThis, name, descriptor);
        else Reflect.deleteProperty(globalThis, name);
      }
    }
  });

  test('logout retires a pending mount and suppresses later online initialisation', async () => {
    const originals = new Map(
      ['window', 'navigator', 'document', 'location', 'performance', 'indexedDB'].map((name) => [
        name,
        Object.getOwnPropertyDescriptor(globalThis, name),
      ]),
    );
    const windowListeners = new Map<string, () => void>();
    const workerListeners = new Map<string, (event: MessageEvent) => void>();
    const messages: unknown[] = [];
    let registrations = 0;
    let failDelete = true;
    let ready!: (value: unknown) => void;
    let stop: (() => void) | undefined;
    let stopNew: (() => void) | undefined;
    const worker = { postMessage: (value: unknown) => messages.push(value) };
    const serviceWorker = {
      register: async () => {
        registrations++;
        return { active: worker };
      },
      ready: new Promise((resolve) => {
        ready = resolve;
      }),
      controller: worker,
      addEventListener: (name: string, listener: (event: MessageEvent) => void) => {
        workerListeners.set(name, listener);
      },
      removeEventListener: (name: string) => workerListeners.delete(name),
    };
    const replace = (name: string, value: unknown): void => {
      Object.defineProperty(globalThis, name, { value, configurable: true });
    };
    try {
      replace('window', {
        Capacitor: {},
        addEventListener: (name: string, listener: () => void) =>
          windowListeners.set(name, listener),
        removeEventListener: (name: string) => windowListeners.delete(name),
      });
      replace('navigator', { serviceWorker });
      replace('document', { querySelectorAll: () => [] });
      replace('location', { href: 'https://suite.invalid/mail' });
      replace('performance', { getEntriesByType: () => [] });
      replace('indexedDB', {
        deleteDatabase: () => {
          if (failDelete) throw new Error('delete failed');
          const request: { onsuccess?: () => void } = {};
          queueMicrotask(() => request.onsuccess?.());
          return request;
        },
      });
      const { activateNativeOffline } = await import('./native.js');
      stop = activateNativeOffline({
        ownerId: 'ada',
        tenantId: 'acme',
        product: 'mail',
        scopeId: 'old-mount',
      });
      workerListeners.get('message')?.({ data: { type: 'SYTUE_CLEAR_OFFLINE' } } as MessageEvent);
      ready({ active: worker });
      windowListeners.get('online')?.();
      // Let the rejected cleanup and pending registration settle. The cleanup
      // rejection must be observed, even though logout cannot wait for deletion.
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(registrations).toBe(1);
      expect(messages).toEqual([]);
      stop();
      expect(messages).toEqual([{ type: 'INVALIDATE', scopeId: 'old-mount' }]);

      // A distinct mount can currently initialise even while cleanup has
      // failed. This test records the limitation; it is not a session fence.
      messages.length = 0;
      stopNew = activateNativeOffline({
        ownerId: 'bea',
        tenantId: 'acme',
        product: 'mail',
        scopeId: 'new-mount',
      });
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(messages).toEqual([
        expect.objectContaining({ type: 'INITIALISE', ownerId: 'bea', scopeId: 'new-mount' }),
      ]);
      stopNew();
      failDelete = false;
      await within(clearOfflineStorage());
    } finally {
      // A failed cleanup closes the storage gate. Bound the retry and always
      // restore globals, even if a regression leaves deletion unresolved.
      failDelete = false;
      try {
        await within(clearOfflineStorage().catch(() => undefined));
        await within(clearOfflineStorage());
      } finally {
        stopNew?.();
        stop?.();
        ready({ active: worker });
        for (const [name, descriptor] of originals) {
          if (descriptor) Object.defineProperty(globalThis, name, descriptor);
          else Reflect.deleteProperty(globalThis, name);
        }
      }
    }
  });
});
