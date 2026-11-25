/* SPDX-License-Identifier: Apache-2.0 */
/**
 * EL WORKER YA NO HACE TRES VIAJES EN SERIE (roadmap, «El núcleo empieza a bajar
 * tarde»): con los resúmenes que el servidor dejó en la página pide `wasm_exec.js` y
 * `core.wasm` A LA VEZ y sin manifiesto. Si esos resúmenes ya no son los del servidor
 * (se desplegó otro núcleo; el servidor contesta 412), pregunta al manifiesto una vez y
 * carga los nuevos. Y sigue sin instanciar un byte que no tenga el sha256 esperado.
 *
 * Visto en ROJO: con `load` esperando a `runtime` antes de pedir el wasm, el primer
 * `fetch` llega después del `import` y el orden de abajo no se cumple.
 */
import { afterAll, expect, test } from 'bun:test';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

// `wasm_exec.js` sale de la red; aquí, de un directorio con la MISMA ruta
// (`…/api/client-core/`) y un Go mínimo que publica el núcleo al arrancar. Un origen
// `file:` es opaco («null»), como el de la `location` de abajo.
const raiz = mkdtempSync(join(tmpdir(), 'sytue-nucleo-'));
mkdirSync(join(raiz, 'api', 'client-core'), { recursive: true });
writeFileSync(
  join(raiz, 'api', 'client-core', 'wasm_exec.js'),
  `globalThis.Go = class {
    constructor() { this.env = {}; this.importObject = {}; }
    run() {
      globalThis.sytuePqc = { version: 1, module_id: 'core', invoke: () => ({ ok: true }) };
      return new Promise(() => {});
    }
  };
  globalThis.__pasosNucleo?.push('import wasm_exec.js');`,
);
const base = pathToFileURL(join(raiz, 'api', 'client-core') + '/').href;
const origin = new URL(base).origin;
// El módulo wasm más pequeño que existe: cabecera y versión.
const nuevo = new Uint8Array([0, 97, 115, 109, 1, 0, 0, 0]);
const sha = (bytes: Uint8Array) => createHash('sha256').update(bytes).digest('hex');
const viejo = 'c'.repeat(64);
const runtime = 'd'.repeat(64);

const pasos: string[] = [];
const publicado: unknown[] = [];
(globalThis as { __pasosNucleo?: string[] }).__pasosNucleo = pasos;

const original = {
  fetch: globalThis.fetch,
  postMessage: (globalThis as { postMessage?: unknown }).postMessage,
  onmessage: (globalThis as { onmessage?: unknown }).onmessage,
  location: (globalThis as { location?: unknown }).location,
};
afterAll(() => {
  Object.assign(globalThis, original);
  rmSync(raiz, { recursive: true, force: true });
});

test('con los resúmenes de la página: wasm_exec.js y core.wasm a la vez, sin manifiesto, y el 412 vuelve al manifiesto', async () => {
  Object.assign(globalThis, {
    location: { origin },
    postMessage: (value: unknown) => publicado.push(value),
    fetch: async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const nombre = url.pathname.split('/').pop()!;
      pasos.push(`fetch ${nombre}${url.search}`);
      if (nombre === 'manifest.json')
        return Response.json({
          version: 1,
          module_id: 'core',
          wasm_sha256: sha(nuevo),
          runtime_sha256: runtime,
        });
      expect(init?.credentials).toBe('same-origin');
      // El servidor solo sirve el sha256 que es el suyo.
      if (url.searchParams.get('sha256') !== sha(nuevo)) return new Response(null, { status: 412 });
      return new Response(nuevo, { headers: { 'content-type': 'application/wasm' } });
    },
  });
  await import('./worker.js');
  const onmessage = (globalThis as unknown as { onmessage: (event: unknown) => void }).onmessage;
  onmessage({
    origin: '',
    data: {
      type: 'initialize',
      assetBaseURL: base,
      module: 'core',
      digests: { wasm_sha256: viejo, runtime_sha256: runtime },
    },
  });
  for (
    let i = 0;
    i < 200 && !publicado.some((m) => (m as { type?: string }).type !== 'progress');
    i++
  )
    await new Promise((r) => setTimeout(r, 5));
  expect(publicado.filter((m) => (m as { type?: string }).type !== 'progress')).toEqual([
    { type: 'ready', module_id: 'core' },
  ]);
  // Lo primero que sale son las dos descargas a la vez; el manifiesto, solo tras el 412.
  // El wasm se pide ANTES de que `wasm_exec.js` llegue a evaluarse: no lo espera.
  expect(pasos.slice(0, 2)).toEqual([`fetch core.wasm?sha256=${viejo}`, 'import wasm_exec.js']);
  expect(pasos.slice(2)).toEqual(['fetch manifest.json', `fetch core.wasm?sha256=${sha(nuevo)}`]);
});
