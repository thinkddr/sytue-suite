/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Copiar formato» (editor/brocha.ts): la receta pura que iguala el formato de destino al
 * copiado, y el ciclo de la brocha sobre un folio —armarse, pintar al soltar el ratón con
 * algo seleccionado, no gastarse con un clic suelto, Esc—. happy-dom no implementa
 * `execCommand`: lo que se pinta lo dice un doble del documento que apunta cada comando.
 *   cd apps/frontend && bun test packages/office/test/docs-brocha.test.ts
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, describe, expect, test } from 'bun:test';
import type { FormatoDeCaracter } from '../src/editor/canvas/exec';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { normalizarColor, pasosDeFormato, setEditorRoot } =
  await import('../src/editor/canvas/exec');
const { alternarBrocha, brochaArmada, desarmarBrocha, oirBrocha } =
  await import('../src/editor/brocha');
type Formato = FormatoDeCaracter;

const LISO: Formato = {
  bold: false,
  italic: false,
  underline: false,
  strike: false,
  superscript: false,
  subscript: false,
  font: 'Inter',
  size: '3',
  color: 'rgb(10, 10, 10)',
  highlight: '',
};

describe('pasosDeFormato', () => {
  test('igual que el destino: nada que hacer', () => {
    expect(pasosDeFormato(LISO, LISO)).toEqual([]);
  });

  test('pone lo que falta, con su valor', () => {
    const fuente: Formato = {
      ...LISO,
      bold: true,
      italic: true,
      font: '"Times New Roman"',
      size: '5',
      color: 'rgb(220, 38, 38)',
      highlight: 'rgb(254, 240, 138)',
    };
    expect(pasosDeFormato(fuente, LISO)).toEqual([
      ['bold'],
      ['italic'],
      ['font', '"Times New Roman"'],
      ['size', '5'],
      ['color', 'rgb(220, 38, 38)'],
      ['highlight', 'rgb(254, 240, 138)'],
    ]);
  });

  test('un título ya es negrita: copiar texto normal sobre él la QUITA', () => {
    expect(pasosDeFormato(LISO, { ...LISO, bold: true })).toEqual([['bold']]);
  });

  test('apaga antes de encender: el superíndice no vuelve a encender el subíndice', () => {
    expect(pasosDeFormato({ ...LISO, superscript: true }, { ...LISO, subscript: true })).toEqual([
      ['subscript'],
      ['superscript'],
    ]);
  });

  test('la misma familia con o sin comillas no se reescribe', () => {
    expect(
      pasosDeFormato({ ...LISO, font: '"Courier New"' }, { ...LISO, font: 'Courier New' }),
    ).toEqual([]);
  });

  test('lo que el navegador no sabe decir no se escribe', () => {
    expect(pasosDeFormato({ ...LISO, font: '', size: '', color: '' }, LISO)).toEqual([]);
  });

  test('un destino resaltado sin resalte en la fuente se limpia', () => {
    expect(pasosDeFormato(LISO, { ...LISO, highlight: 'rgb(254, 240, 138)' })).toEqual([
      ['highlight', 'transparent'],
    ]);
  });
});

test('normalizarColor: transparente es sin resalte; el resto, sin espacios raros', () => {
  expect(normalizarColor('rgba(0, 0, 0, 0)')).toBe('');
  expect(normalizarColor('transparent')).toBe('');
  expect(normalizarColor('')).toBe('');
  expect(normalizarColor('rgb(254,240, 138)')).toBe('rgb(254, 240, 138)');
});

describe('el ciclo de la brocha', () => {
  let raiz: HTMLElement;
  let mandados: string[];
  const doc = document as unknown as Record<string, unknown>;
  const originales = {
    execCommand: doc['execCommand'],
    queryCommandState: doc['queryCommandState'],
    queryCommandValue: doc['queryCommandValue'],
  };

  beforeEach(() => {
    mandados = [];
    // El origen («hola») está en negrita; el resto, no.
    doc['queryCommandState'] = (c: string): boolean =>
      c === 'bold' && (document.getSelection()?.toString() ?? '') === 'hola';
    doc['queryCommandValue'] = (): string => '';
    doc['execCommand'] = (c: string, _ui: boolean, v?: string): boolean => {
      if (c !== 'styleWithCSS' && c !== 'defaultParagraphSeparator')
        mandados.push(v === undefined ? c : `${c}:${v}`);
      return true;
    };
    raiz = document.createElement('div');
    raiz.contentEditable = 'true';
    raiz.innerHTML = '<p><b>hola</b> mundo cruel</p>';
    document.body.append(raiz);
    setEditorRoot(raiz);
    raiz.focus();
  });
  afterEach(() => {
    desarmarBrocha();
    setEditorRoot(null);
    raiz.remove();
    Object.assign(doc, originales);
  });

  const seleccionar = (texto: Node, desde: number, hasta: number): void => {
    const r = document.createRange();
    r.setStart(texto, desde);
    r.setEnd(texto, hasta);
    const sel = document.getSelection()!;
    sel.removeAllRanges();
    sel.addRange(r);
  };
  const soltar = async (): Promise<void> => {
    raiz.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
    await new Promise((r) => setTimeout(r, 1));
  };

  test('se arma, un clic suelto no la gasta y al soltar sobre una selección pinta', async () => {
    let avisos = 0;
    const dejar = oirBrocha(() => (avisos += 1));
    const origen = raiz.querySelector('b')!.firstChild!;
    seleccionar(origen, 0, 4);
    alternarBrocha();
    expect(brochaArmada()).toBe(true);
    expect(raiz.hasAttribute('data-brocha')).toBe(true); // el puntero de copiar

    const destino = raiz.querySelector('p')!.lastChild!; // « mundo cruel»
    seleccionar(destino, 3, 3); // un clic: cursor colapsado
    await soltar();
    expect(brochaArmada()).toBe(true);
    expect(mandados).toEqual([]);

    seleccionar(destino, 1, 6); // «mundo»
    await soltar();
    expect(brochaArmada()).toBe(false);
    expect(raiz.hasAttribute('data-brocha')).toBe(false);
    // «Borrar formato» y la negrita que falta.
    expect(mandados).toEqual(['removeFormat', 'bold']);
    expect(avisos).toBe(2);
    dejar();
  });

  test('Esc la desarma sin pintar', async () => {
    seleccionar(raiz.querySelector('b')!.firstChild!, 0, 4);
    alternarBrocha();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(brochaArmada()).toBe(false);
    seleccionar(raiz.querySelector('p')!.lastChild!, 1, 6);
    await soltar();
    expect(mandados).toEqual([]);
  });

  test('con el teclado: otra pulsación pinta lo seleccionado; sobre el origen, la desarma', () => {
    const origen = raiz.querySelector('b')!.firstChild!;
    seleccionar(origen, 0, 4);
    alternarBrocha();
    alternarBrocha(); // la misma selección: se arrepiente
    expect(brochaArmada()).toBe(false);
    expect(mandados).toEqual([]);

    alternarBrocha();
    seleccionar(raiz.querySelector('p')!.lastChild!, 7, 12); // «cruel»
    alternarBrocha();
    expect(brochaArmada()).toBe(false);
    expect(mandados).toEqual(['removeFormat', 'bold']);
  });
});
