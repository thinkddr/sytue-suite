/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL EDITOR SE PARTE POR TIPO DE DOCUMENTO (`src/editor/tipos`). La isla (`EditorApp`)
 * no lleva ningún lienzo ni ninguna pestaña: cada tipo baja los suyos. Y un documento o
 * una presentación no bajan el lienzo de hoja ni el motor de fórmulas; ninguno baja
 * parse5 hasta que se abre lo que lo usa (los scripts, «Ver respuestas»).
 *
 * Se mira el grafo de importaciones ESTÁTICAS (las de tipo y los `import()` no cuentan)
 * siguiendo los ficheros del monorepo: lo que el navegador baja sin pedir nada.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { cargarEditorDe, editorCargado } from '../src/editor/tipos';

function staticGraph(entry: string): string[] {
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
      try {
        pending.push(Bun.resolveSync(item.path, dirname(file)));
      } catch {
        /* un CSS o un JSON sin resolver: no es JS */
      }
    }
  }
  return [...seen];
}

const grafo = (ruta: string): string[] => staticGraph(Bun.resolveSync(ruta, import.meta.dir));
const hay = (g: string[], re: RegExp): string[] => g.filter((f) => re.test(f));

const LIENZO_HOJA = /\/editor\/sheets\/SheetCanvas\.tsx$/;
const LIENZO_DIAPOS = /\/editor\/slides\/SlidesCanvas\.tsx$/;
const FOLIO = /\/editor\/canvas\/PageCanvas\.tsx$/;
const PESTANAS = /\/editor\/ribbon\/tabs\//;
const FORMULAS = /\/packages\/formula\/src\/(engine|functions\/)/;
const PARSE5 = /\/node_modules\/.*\/(parse5|entities)\//;

test('the editor island carries no canvas, no tab body, no formulas and no parse5', () => {
  const g = grafo('../src/editor/EditorApp.tsx');
  expect(g.some((f) => f.endsWith('/editor/ribbon/Ribbon.tsx'))).toBe(true);
  for (const re of [LIENZO_HOJA, LIENZO_DIAPOS, FOLIO, PESTANAS, FORMULAS, PARSE5])
    expect({ re: String(re), files: hay(g, re) }).toEqual({ re: String(re), files: [] });
});

test('each kind carries its own canvas, and only the spreadsheet the formula engine', () => {
  const documento = grafo('../src/editor/tipos/documento.tsx');
  const hoja = grafo('../src/editor/tipos/hoja.tsx');
  const presentacion = grafo('../src/editor/tipos/presentacion.tsx');
  expect(hay(documento, FOLIO)).toHaveLength(1);
  expect(hay(hoja, LIENZO_HOJA)).toHaveLength(1);
  expect(hay(presentacion, LIENZO_DIAPOS)).toHaveLength(1);
  expect(hay(hoja, FORMULAS).length).toBeGreaterThan(5);
  for (const g of [documento, presentacion]) {
    expect(hay(g, LIENZO_HOJA)).toEqual([]);
    expect(hay(g, FORMULAS)).toEqual([]);
  }
  expect(hay(documento, LIENZO_DIAPOS)).toEqual([]);
  expect(hay(presentacion, FOLIO)).toEqual([]);
  // parse5 baja con los scripts y con «Ver respuestas», al abrirlos.
  for (const g of [documento, hoja, presentacion]) expect(hay(g, PARSE5)).toEqual([]);
});

test('the kind editor loads once and is then available without waiting', async () => {
  const [a, b] = await Promise.all([cargarEditorDe('document'), cargarEditorDe('document')]);
  expect(b).toBe(a);
  expect(editorCargado('document')).toBe(a);
  expect(Object.keys(a.cuerpos)).toContain('referencias');
  expect(a.atajos(((k: string) => k) as never)).toEqual([]);
});
