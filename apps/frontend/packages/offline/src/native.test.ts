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
