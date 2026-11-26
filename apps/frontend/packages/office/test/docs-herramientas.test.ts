/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo nuevo del folio de Docus contra los VECTORES COMPARTIDOS con la app nativa
 * (`vectores/docus-herramientas.json`, que Go recorre en
 * `apps/fyne/editor/docus/herramientas_test.go`) y sobre un folio de verdad: ordenar
 * párrafos, Mayús+F3, aumentar y reducir la letra, el tiempo de lectura, el texto sin
 * formato y la fecha y hora.
 *   cd apps/frontend && bun test packages/office/test/docs-herramientas.test.ts
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { siguienteModoMayusculas, type ModoMayusculas } from '../src/editor/mayusculas';
import { minutosDeLectura, textoPlanoDeDescarga } from '../src/editor/recuento';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { exec, ordenDeTextos, pasoDeTalla, setEditorRoot } =
  await import('../src/editor/canvas/exec');
const { atenderAtajoDeTexto } = await import('../src/editor/canvas/atajos');
const { formatosFechaHora } = await import('../src/editor/ribbon/tabs/Insertar');

interface Vectores {
  ordenar: { idioma: string; textos: string[]; asc: number[]; desc: number[] }[];
  ciclo: { texto: string; sale: ModoMayusculas }[];
  talla: { actual: string; dir: 1 | -1; sale: string }[];
  lectura: { palabras: number; sale: number }[];
  textoPlano: { texto: string; sale: string }[];
}
const v = JSON.parse(
  readFileSync(new URL('./vectores/docus-herramientas.json', import.meta.url), 'utf8'),
) as Vectores;

describe('los vectores compartidos con la app nativa', () => {
  for (const caso of v.ordenar) {
    test(`ordenar ${JSON.stringify(caso.textos)}`, () => {
      expect(ordenDeTextos(caso.textos, false, caso.idioma)).toEqual(caso.asc);
      expect(ordenDeTextos(caso.textos, true, caso.idioma)).toEqual(caso.desc);
    });
  }
  for (const caso of v.ciclo) {
    test(`Mayús+F3 sobre ${JSON.stringify(caso.texto)}`, () => {
      expect(siguienteModoMayusculas(caso.texto)).toBe(caso.sale);
    });
  }
  test('el peldaño de letra', () => {
    for (const caso of v.talla) expect(pasoDeTalla(caso.actual, caso.dir)).toBe(caso.sale);
  });
  test('el tiempo de lectura', () => {
    for (const caso of v.lectura) expect(minutosDeLectura(caso.palabras)).toBe(caso.sale);
  });
  test('el texto sin formato', () => {
    for (const caso of v.textoPlano) expect(textoPlanoDeDescarga(caso.texto)).toBe(caso.sale);
  });
});

describe('sobre el folio', () => {
  let folio: HTMLElement | null = null;
  afterEach(() => {
    folio?.remove();
    folio = null;
    setEditorRoot(null);
  });
  const montar = (html: string): HTMLElement => {
    folio = document.createElement('div');
    folio.contentEditable = 'true';
    folio.innerHTML = html;
    document.body.appendChild(folio);
    setEditorRoot(folio);
    return folio;
  };
  const elegir = (desde: Node, hasta: Node): void => {
    const r = document.createRange();
    r.setStart(desde, 0);
    r.setEnd(hasta, hasta.childNodes.length);
    const sel = document.getSelection()!;
    sel.removeAllRanges();
    sel.addRange(r);
  };

  test('ordena los párrafos elegidos y deja los de fuera donde estaban', () => {
    const f = montar('<p>fuera</p><p>pera</p><p>Árbol</p><p>manzana</p><h2>título</h2>');
    elegir(f.children[1]!, f.children[3]!);
    exec('sortParagraphs', 'asc');
    expect(Array.from(f.children).map((c) => c.textContent)).toEqual([
      'fuera',
      'Árbol',
      'manzana',
      'pera',
      'título',
    ]);
    exec('sortParagraphs', 'desc');
    expect(Array.from(f.children).map((c) => c.textContent)).toEqual([
      'fuera',
      'pera',
      'manzana',
      'Árbol',
      'título',
    ]);
  });

  test('no mezcla una lista con los párrafos de alrededor', () => {
    const f = montar('<p>z</p><ul><li>b</li><li>a</li></ul>');
    const antes = f.innerHTML;
    elegir(f.children[0]!, f.querySelector('li:last-child')!);
    exec('sortParagraphs', 'asc');
    expect(f.innerHTML).toBe(antes);
  });

  test('Mayús+F3 sobre lo elegido despacha el modo siguiente del ciclo', () => {
    const f = montar('<p>hola mundo</p>');
    elegir(f.children[0]!, f.children[0]!);
    const pedidos: [string, string | undefined][] = [];
    const tecla = {
      key: 'F3',
      shiftKey: true,
      ctrlKey: false,
      metaKey: false,
      altKey: false,
      preventDefault: () => {},
    };
    expect(atenderAtajoDeTexto(tecla, { exec: (c, val) => pedidos.push([c, val]) })).toBe(true);
    expect(pedidos).toEqual([['changeCase', 'upper']]);
    // En la celda de Calcus no es suyo.
    expect(atenderAtajoDeTexto(tecla, { exec: () => {} }, new Set(['bold']))).toBe(false);
  });

  test('la fecha y la hora se ofrecen ya escritas, en el idioma de quien escribe', () => {
    const fecha = new Date(2026, 10, 25, 9, 7);
    const es = formatosFechaHora('es', fecha);
    expect(es).toHaveLength(4);
    expect(es[1]).toBe('25 de noviembre de 2026');
    expect(es[2]).toBe('miércoles, 25 de noviembre de 2026');
    expect(es[3]).toBe('9:07');
    expect(new Set(formatosFechaHora('de', fecha)).size).toBe(4);
  });
});
