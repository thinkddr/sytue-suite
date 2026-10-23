import { describe, expect, test } from 'bun:test';
import { currentConnectivity } from './native.js';

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
});
