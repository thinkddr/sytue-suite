/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';

// Un Rooms anterior al núcleo de la bóveda contesta 400 a `?module=vault`: el worker
// pide entonces el manifiesto del núcleo completo, y solo en ese caso. Se ejecuta el
// worker de verdad hasta el manifiesto; lo que sigue (el wasm) no hace falta aquí.
async function manifests(module: string, status: number): Promise<string[]> {
  const host = globalThis as unknown as Record<string, unknown>;
  const saved = {
    fetch: host.fetch,
    postMessage: host.postMessage,
    location: host.location,
    onmessage: host.onmessage,
    sytueCoreHostCall: host.sytueCoreHostCall,
  };
  const pedidos: string[] = [];
  let failed!: () => void;
  const done = new Promise<void>((resolve) => (failed = resolve));
  host.location = { origin: 'https://app.sytue.test' };
  host.postMessage = (message: { type?: string }) => {
    if (message.type === 'failed') failed();
  };
  host.fetch = (input: URL) => {
    pedidos.push(input.pathname.slice(input.pathname.lastIndexOf('/') + 1) + input.search);
    return Promise.resolve(new Response(null, { status: pedidos.length === 1 ? status : 503 }));
  };
  try {
    await import('./worker.ts?' + module + status);
    (host.onmessage as (event: unknown) => void)({
      origin: '',
      data: {
        type: 'initialize',
        assetBaseURL: 'https://app.sytue.test/drive/api/client-core/',
        module,
      },
    });
    await done;
    return pedidos;
  } finally {
    Object.assign(host, saved);
  }
}

test('vault cae al núcleo completo solo si el servidor no conoce el módulo', async () => {
  expect(await manifests('vault', 400)).toEqual(['manifest.json?module=vault', 'manifest.json']);
  expect(await manifests('vault', 503)).toEqual(['manifest.json?module=vault']);
  expect(await manifests('office', 400)).toEqual(['manifest.json?module=office']);
});
