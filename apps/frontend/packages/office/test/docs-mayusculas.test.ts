/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Cambiar mayúsculas y minúsculas» del folio: la transformación pura contra los
 * VECTORES COMPARTIDOS con la app nativa (`vectores/docus-mayusculas.json`, que Go
 * recorre en `apps/fyne/editor/docus/rico/mayusculas_test.go`) y `exec('changeCase')`
 * sobre un folio de verdad: el marcado no se mueve y la selección sigue abarcando lo mismo.
 *   cd apps/frontend && bun test packages/office/test/docs-mayusculas.test.ts
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  cambioDeMayusculas,
  esModoMayusculas,
  type ModoMayusculas,
} from '../src/editor/mayusculas';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { exec, setEditorRoot } = await import('../src/editor/canvas/exec');

interface Caso {
  nombre: string;
  modo: ModoMayusculas;
  previo?: string;
  parrafos: string[][];
  esperado: string[][];
}
const vectores = JSON.parse(
  readFileSync(new URL('./vectores/docus-mayusculas.json', import.meta.url), 'utf8'),
) as { casos: Caso[] };

describe('los vectores compartidos con la app nativa', () => {
  test('hay casos de los cuatro modos', () => {
    expect(new Set(vectores.casos.map((c) => c.modo))).toEqual(
      new Set(['sentence', 'lower', 'upper', 'title']),
    );
  });
  for (const caso of vectores.casos) {
    test(caso.nombre, () => {
      const cambio = cambioDeMayusculas(caso.modo);
      const obtenido = caso.parrafos.map((trozos, i) => {
        cambio.parrafo();
        if (i === 0 && caso.previo) cambio.previo(caso.previo);
        return trozos.map((t) => cambio.tramo(t));
      });
      expect(obtenido).toEqual(caso.esperado);
    });
  }
});

test('un modo que no es de los cuatro no se acepta', () => {
  expect(esModoMayusculas('title')).toBe(true);
  expect(esModoMayusculas('alternar')).toBe(false);
  expect(esModoMayusculas(undefined)).toBe(false);
});

describe("exec('changeCase') en el folio", () => {
  let raiz: HTMLElement;
  const folio = (html: string): HTMLElement => {
    raiz = document.createElement('div');
    raiz.contentEditable = 'true';
    raiz.innerHTML = html;
    document.body.append(raiz);
    setEditorRoot(raiz);
    raiz.focus();
    return raiz;
  };
  afterEach(() => {
    setEditorRoot(null);
    raiz.remove();
  });
  const seleccionar = (inicio: Node, desde: number, fin: Node, hasta: number): void => {
    const r = document.createRange();
    r.setStart(inicio, desde);
    r.setEnd(fin, hasta);
    const sel = document.getSelection()!;
    sel.removeAllRanges();
    sel.addRange(r);
  };

  test('cruza nodos y párrafos sin mover el formato ni la selección', () => {
    const f = folio('<p>hola <b>mUNDO</b> cruel</p><p>otra cosa</p>');
    const p1 = f.querySelector('p')!;
    const p2 = f.querySelectorAll('p')[1]!;
    let entradas = 0;
    f.addEventListener('input', () => (entradas += 1));
    // De «la» (en «hola») a «otra» incluida.
    seleccionar(p1.firstChild!, 2, p2.firstChild!, 4);
    exec('changeCase', 'title');
    // «la» sigue la palabra «ho», que no está seleccionada: no empieza palabra. El
    // segundo párrafo, sí.
    expect(f.innerHTML).toBe('<p>hola <b>Mundo</b> Cruel</p><p>Otra cosa</p>');
    expect(document.getSelection()!.toString()).toBe('la Mundo CruelOtra');
    expect(entradas).toBe(1); // el autosave se entera
  });

  test('MAYÚSCULAS respeta lo no editable (una cita la escribe el editor)', () => {
    const f = folio('<p>ver <span contenteditable="false">(ruiz, 2020)</span> aquí</p>');
    const p = f.querySelector('p')!;
    seleccionar(p, 0, p, p.childNodes.length);
    exec('changeCase', 'upper');
    expect(f.innerHTML).toBe('<p>VER <span contenteditable="false">(ruiz, 2020)</span> AQUÍ</p>');
  });

  test('sin selección, o con un modo inventado, no hace nada', () => {
    const f = folio('<p>Hola</p>');
    const texto = f.querySelector('p')!.firstChild!;
    seleccionar(texto, 2, texto, 2);
    exec('changeCase', 'upper');
    seleccionar(texto, 0, texto, 4);
    exec('changeCase', 'alternar');
    expect(f.innerHTML).toBe('<p>Hola</p>');
  });
});
