// El service worker abre la ventana que diga el aviso push. Este test lo carga tal
// cual (es un script suelto en `public/`, sin módulos) con un `self` de mentira y
// mira a dónde iría al pulsar una notificación.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const codigo = readFileSync(fileURLToPath(new URL('../../public/sw.js', import.meta.url)), 'utf8');

async function alPulsar(url: unknown): Promise<string> {
  const abiertas: string[] = [];
  const manejadores: Record<string, (e: unknown) => void> = {};
  const self = {
    location: { origin: 'https://rooms.example', pathname: '/rooms/sw.js' },
    addEventListener: (tipo: string, fn: (e: unknown) => void) => {
      manejadores[tipo] = fn;
    },
    clients: {
      matchAll: async () => [],
      openWindow: async (u: string) => {
        abiertas.push(u);
      },
    },
  };
  vm.runInNewContext(codigo, { self, URL });
  let espera: Promise<unknown> = Promise.resolve();
  manejadores['notificationclick']!({
    notification: { close: () => {}, data: { url } },
    waitUntil: (p: Promise<unknown>) => {
      espera = p;
    },
  });
  await espera;
  expect(abiertas.length).toBe(1);
  return abiertas[0]!;
}

test('una ruta de la app se abre tal cual', async () => {
  expect(await alPulsar('/?channel=c1&message=m1')).toBe(
    'https://rooms.example/?channel=c1&message=m1',
  );
});

test('lo que no cae en este origen no se sigue: se abre la app', async () => {
  for (const hostil of [
    'https://atacante.example/entrar',
    '//atacante.example/entrar',
    'javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
  ]) {
    expect(await alPulsar(hostil)).toBe('https://rooms.example/');
  }
});
