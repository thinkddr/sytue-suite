/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { cargarHtml, htmlCargado, parseFragment, textoPegado } from './html.js';
import { listNotebooks } from './notesApi.js';

/**
 * Lo que la isla baja SIN pedir nada: las importaciones estáticas (las de tipo y los
 * `import()` no cuentan) desde la entrada, siguiendo los ficheros del monorepo. Si
 * parse5 o el saneador del pegado vuelven a estar aquí, vuelven al JS inicial.
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

const html = (file: string): boolean =>
  /\/(parse5|entities)\//.test(file) || file.endsWith('/browser/portapapeles.ts');

test('the private island does not reach parse5 or the paste sanitizer statically', () => {
  const graph = [
    ...staticGraph(Bun.resolveSync('../components/PrivateNotesApp.tsx', import.meta.dir)),
  ];
  expect(graph.some((file) => file.endsWith('/components/NotesApp.tsx'))).toBe(true);
  expect(graph.some((file) => file.endsWith('/client/extracto.ts'))).toBe(true);
  expect(graph.filter(html)).toEqual([]);
});

test('the public island, painted on the server with its notes, keeps parse5 static', () => {
  const graph = [
    ...staticGraph(Bun.resolveSync('../components/PublicNotebookIsla.tsx', import.meta.dir)),
  ];
  expect(graph.some((file) => file.includes('/parse5/'))).toBe(true);
});

const savedFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = savedFetch;
});

test('no BFF answer reaches the interface before the parser', async () => {
  globalThis.fetch = (async () => Response.json({ notebooks: [] })) as unknown as typeof fetch;
  expect(await listNotebooks()).toEqual([]);
  expect(htmlCargado()).toBe(true);
  expect(parseFragment('<p>uno <b>dos</b></p>').childNodes).toHaveLength(1);
  const datos = {
    getData: (tipo: string) => (tipo === 'text/html' ? '<p>de&nbsp;Safari</p>' : ''),
  };
  expect(textoPegado(datos)).toBe('de Safari');
  // Una vez cargado, cargar otra vez no espera nada.
  await cargarHtml();
});
