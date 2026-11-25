/* SPDX-License-Identifier: Apache-2.0 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { MatrixBootstrap } from '@sytue/offline/matrix';
import type { MatrixGateHost } from './MatrixDeviceGate.js';
import { MatrixDeviceGate, loadMatrixBranch } from './matrix-lazy.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

/**
 * Lo que una app de la bóveda baja SIN pedir la rama de Matrix: las importaciones
 * estáticas (las de tipo y los `import()` no cuentan) desde la entrada, siguiendo los
 * ficheros del monorepo. Si matrix-js-sdk o la puerta de dispositivos vuelven a estar
 * aquí, vuelven al JS inicial de cada app.
 */
function staticGraph(entry: string): Set<string> {
  const transpilers = {
    tsx: new Bun.Transpiler({ loader: 'tsx', trimUnusedImports: true }),
    ts: new Bun.Transpiler({ loader: 'ts', trimUnusedImports: true }),
  };
  const seen = new Set<string>();
  const pending = [entry];
  while (pending.length) {
    const file = pending.pop()!;
    if (seen.has(file)) continue;
    seen.add(file);
    if (file.includes('/node_modules/') || !/\.(ts|tsx|js|mjs)$/.test(file)) continue;
    const transpiler = file.endsWith('.tsx') ? transpilers.tsx : transpilers.ts;
    for (const item of transpiler.scanImports(readFileSync(file, 'utf8'))) {
      if (item.kind !== 'import-statement') continue;
      let resolved: string;
      try {
        resolved = Bun.resolveSync(item.path, dirname(file));
      } catch {
        continue;
      }
      pending.push(resolved);
    }
  }
  return seen;
}

test('@sytue/shell/vault does not reach matrix-js-sdk or the device gate statically', () => {
  const graph = [...staticGraph(Bun.resolveSync('@sytue/shell/vault', import.meta.dir))];
  expect(graph.some((file) => file.endsWith('/matrix/MatrixSourceGate.tsx'))).toBe(true);
  expect(graph.filter((file) => file.includes('/matrix-js-sdk/'))).toEqual([]);
  expect(graph.filter((file) => /\/(MatrixDeviceGate\.tsx|matrix-branch\.ts)$/.test(file))).toEqual(
    [],
  );
  expect(graph.filter((file) => file.endsWith('/offline/src/matrix/client.ts'))).toEqual([]);
});

test('the vault entries do not reach the Matrix attachment crypto statically', () => {
  // `@matrix-org/matrix-sdk-crypto-wasm` (~244 KB de JS) solo hace falta al cifrar o
  // descifrar un bloque de fichero: lo baja `attachment-wasm.ts` con `import()`.
  const entries = [
    '@sytue/shell/vault',
    '@sytue/offline/vault',
    '@sytue/offline/vault/drive',
    '@sytue/offline/vault/mail',
    '@sytue/offline/matrix',
  ];
  for (const entry of entries) {
    const graph = [...staticGraph(Bun.resolveSync(entry, import.meta.dir))];
    expect({
      entry,
      crypto: graph.filter((file) => file.includes('/matrix-sdk-crypto-wasm/')),
    }).toEqual({
      entry,
      crypto: [],
    });
  }
  const storage = [...staticGraph(Bun.resolveSync('@sytue/offline/vault', import.meta.dir))];
  expect(storage.some((file) => file.endsWith('/offline/src/matrix/storage-files.ts'))).toBe(true);
  expect(storage.some((file) => file.endsWith('/offline/src/matrix/attachment-wasm.ts'))).toBe(
    true,
  );
});

test('the Matrix index keeps the eager gate for Rooms', () => {
  const graph = [...staticGraph(Bun.resolveSync('@sytue/shell/matrix', import.meta.dir))];
  expect(graph.some((file) => file.endsWith('/matrix/MatrixDeviceGate.tsx'))).toBe(true);
});

const actor = '11111111-1111-4111-8111-111111111111';

test('the lazy gate renders the real device gate once its chunk loads', async () => {
  const root = document.createElement('div');
  document.body.append(root);
  let bootstraps = 0;
  const host: MatrixGateHost = {
    translate: (key) => key,
    // Never answers: the real gate stays in its loading state, which is enough to see it.
    bootstrap: () => {
      bootstraps++;
      return new Promise<MatrixBootstrap>(() => {});
    },
    begin: async () => {},
    session: {
      readSession: async () => null,
      forgetSession: async () => {},
      refreshBootstrap: () => new Promise<MatrixBootstrap>(() => {}),
    },
  };
  await act(() => {
    render(
      <MatrixDeviceGate actor={actor} tenant="acme" locale="en" host={host}>
        <p data-private="true">private</p>
      </MatrixDeviceGate>,
      root,
    );
  });
  const branch = await loadMatrixBranch();
  for (let i = 0; i < 100 && bootstraps === 0; i++)
    await act(() => new Promise((r) => setTimeout(r, 10)));
  expect(bootstraps).toBeGreaterThan(0);
  expect(root.querySelector('[data-private]')).toBeNull();
  expect(root.querySelector('main')).not.toBeNull();
  // The same module every time: a second gate mounts without waiting again.
  expect(await loadMatrixBranch()).toBe(branch);
  await act(() => render(null, root));
  root.remove();
});
