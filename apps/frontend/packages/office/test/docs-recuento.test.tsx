/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Recuento de palabras detallado de Docus (editor/recuento.ts y la barra de estado):
 * las cifras de un texto contra los VECTORES COMPARTIDOS con la app nativa
 * (`vectores/docus-recuento.json`, que Go recorre en `apps/fyne/editor/recuento_test.go`),
 * el texto que se ve de un folio, «Palabras: 12 de 340» con selección y el diálogo, que
 * se abre con la cifra o con Ctrl/⌘ Mayús C y que en Diapos no existe.
 *   cd apps/frontend && bun test packages/office/test/docs-recuento.test.tsx
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { CifrasTexto } from '../src/editor/recuento';
import type { EditorContext, PageSetup } from '../src/editor/types';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { docsT } = await import('../src/i18n');
const { setEditorRoot } = await import('../src/editor/canvas/exec');
const { cifrasDelTexto, esAtajoRecuento, textoDeNodo, textoDelHtml } =
  await import('../src/editor/recuento');
const { StatusBar, RECUENTO_THROTTLE_MS } = await import('../src/editor/layout/StatusBar');
const { PAGINA_POR_DEFECTO } = await import('../src/editor/pagedecor');

interface Caso {
  nombre: string;
  texto: string;
  esperado: CifrasTexto;
}
const vectores = JSON.parse(
  readFileSync(new URL('./vectores/docus-recuento.json', import.meta.url), 'utf8'),
) as { casos: Caso[] };

describe('los vectores compartidos con la app nativa', () => {
  for (const caso of vectores.casos) {
    test(caso.nombre, () => {
      expect(cifrasDelTexto(caso.texto)).toEqual(caso.esperado);
    });
  }
});

describe('el texto que se ve', () => {
  test('cada bloque es un párrafo y el blanco de entre etiquetas no cuenta', () => {
    const texto = textoDelHtml(
      '<h1>Título</h1>\n  <p>Uno <strong>dos</strong>\n   tres</p><ul><li>a</li><li>b<br>c</li></ul>' +
        '<script type="application/json">{"no":"cuenta"}</script>',
    );
    expect(texto.split('\n').filter((l) => l !== '')).toEqual([
      'Título',
      'Uno dos tres',
      'a',
      'b',
      'c',
    ]);
    expect(cifrasDelTexto(texto)).toEqual({
      palabras: 7,
      caracteres: 6 + 12 + 1 + 1 + 1,
      caracteresSinEspacios: 6 + 10 + 3,
      parrafos: 5,
    });
  });

  test('un fragmento de selección que corta dos párrafos da dos párrafos', () => {
    const div = document.createElement('div');
    div.innerHTML = '<p>Primero aquí</p><p>Segundo allí</p>';
    const range = document.createRange();
    range.setStart(div.querySelector('p')!.firstChild!, 8);
    range.setEnd(div.querySelectorAll('p')[1]!.firstChild!, 7);
    expect(cifrasDelTexto(textoDeNodo(range.cloneContents()))).toEqual({
      palabras: 2,
      caracteres: 11,
      caracteresSinEspacios: 11,
      parrafos: 2,
    });
  });
});

test('Ctrl/⌘ Mayús C es el atajo; sin Mayús o con Alt no', () => {
  const base = {
    key: 'C',
    code: 'KeyC',
    ctrlKey: true,
    metaKey: false,
    shiftKey: true,
    altKey: false,
  };
  expect(esAtajoRecuento(base)).toBe(true);
  expect(esAtajoRecuento({ ...base, ctrlKey: false, metaKey: true })).toBe(true);
  expect(esAtajoRecuento({ ...base, key: 'С' })).toBe(true); // cirílica, la misma tecla
  expect(esAtajoRecuento({ ...base, shiftKey: false, key: 'c' })).toBe(false); // copiar
  expect(esAtajoRecuento({ ...base, altKey: true })).toBe(false);
  // Dvorak: la tecla física de la C dice «j»; Ctrl+Mayús+J no es el recuento.
  expect(esAtajoRecuento({ ...base, key: 'J' })).toBe(false);
});

/* ---------- la barra de estado ---------- */

let host: HTMLElement | undefined;
let folio: HTMLElement;
/** Lo que la barra le pide a setPageSetup (el idioma del documento). */
let paginas: Partial<PageSetup>[] = [];

function montar(
  kind: 'document' | 'presentation' = 'document',
  pagina: PageSetup = { ...PAGINA_POR_DEFECTO },
  soloLectura = false,
): void {
  paginas = [];
  document.body.innerHTML = '';
  folio = document.createElement('div');
  folio.contentEditable = 'true';
  folio.innerHTML = '<p>Uno dos tres</p><p>cuatro cinco</p>';
  document.body.appendChild(folio);
  setEditorRoot(folio);
  const h = document.createElement('div');
  host = h;
  document.body.appendChild(h);
  const ctx = {
    t: docsT('es'),
    locale: 'es',
    kind,
    content: '<p>Uno dos tres</p><p>cuatro cinco</p>',
    stats: { page: 1, pages: 3, words: 5 },
    ui: { zoom: 100 },
    setUi: () => undefined,
    pageSetup: pagina,
    setPageSetup: (p: Partial<PageSetup>) => paginas.push(p),
    soloLectura,
  } as unknown as EditorContext;
  act(() => {
    render(<StatusBar ctx={ctx} />, h);
  });
}

function seleccionar(desde: number, hasta: number): void {
  const texto = folio.querySelector('p')!.firstChild!;
  const range = document.createRange();
  range.setStart(texto, desde);
  range.setEnd(texto, hasta);
  const sel = document.getSelection()!;
  sel.removeAllRanges();
  sel.addRange(range);
  document.dispatchEvent(new Event('selectionchange'));
}

const esperar = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms));
const botonPalabras = (): HTMLButtonElement | undefined =>
  [...(host?.querySelectorAll('button') ?? [])].find((b) => b.textContent?.startsWith('Palabras'));

afterEach(() => {
  const h = host;
  if (h) {
    act(() => {
      render(null, h);
    });
  }
  document.getSelection()?.removeAllRanges();
  setEditorRoot(null);
});

describe('la barra de estado', () => {
  test('con selección dice «Palabras: 2 de 5», tras el throttle de 100 ms', async () => {
    montar();
    expect(botonPalabras()?.textContent).toBe('Palabras: 5');
    seleccionar(0, 7); // «Uno dos»
    expect(botonPalabras()?.textContent).toBe('Palabras: 5'); // aún dentro del throttle
    await act(() => esperar(RECUENTO_THROTTLE_MS + 30));
    expect(botonPalabras()?.textContent).toBe('Palabras: 2 de 5');
    document.getSelection()!.removeAllRanges();
    document.dispatchEvent(new Event('selectionchange'));
    await act(() => esperar(RECUENTO_THROTTLE_MS + 30));
    expect(botonPalabras()?.textContent).toBe('Palabras: 5');
  });

  test('la cifra es un botón que abre el recuento detallado', () => {
    montar();
    const boton = botonPalabras()!;
    expect(boton.getAttribute('aria-haspopup')).toBe('dialog');
    expect(boton.title).toBe('Recuento de palabras (Ctrl/⌘+Mayús+C)');
    act(() => boton.click());
    const dialogo = document.querySelector('[role="dialog"]');
    expect(dialogo).not.toBeNull();
    const filas = [...dialogo!.querySelectorAll('tbody tr')].map((tr) =>
      [...tr.querySelectorAll('td')].map((td) => td.textContent),
    );
    expect(filas).toEqual([
      ['Páginas', '3'],
      ['Palabras', '5'],
      ['Caracteres', '24'],
      ['Caracteres sin espacios', '21'],
      ['Párrafos', '2'],
    ]);
  });

  test('Ctrl+Mayús+C con selección enseña también la columna de la selección', () => {
    montar();
    seleccionar(4, 12); // «dos tres»
    const ev = new KeyboardEvent('keydown', {
      key: 'C',
      code: 'KeyC',
      ctrlKey: true,
      shiftKey: true,
      cancelable: true,
    });
    act(() => {
      window.dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(true);
    const dialogo = document.querySelector('[role="dialog"]')!;
    expect([...dialogo.querySelectorAll('th')].map((th) => th.textContent)).toEqual([
      'Recuento de palabras',
      'Selección',
      'Documento',
    ]);
    const palabras = [...dialogo.querySelectorAll('tbody tr')][1]!;
    expect([...palabras.querySelectorAll('td')].map((td) => td.textContent)).toEqual([
      'Palabras',
      '2',
      '5',
    ]);
  });

  test('en Diapos no hay recuento: ni cifra ni atajo', () => {
    montar('presentation');
    expect(botonPalabras()).toBeUndefined();
    const ev = new KeyboardEvent('keydown', {
      key: 'C',
      ctrlKey: true,
      shiftKey: true,
      cancelable: true,
    });
    act(() => {
      window.dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(false);
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });
});

/* ---------- el idioma del documento ---------- */

const selectorIdioma = (): HTMLSelectElement =>
  host!.querySelector<HTMLSelectElement>('select[aria-label="Idioma del documento"]')!;

describe('el idioma del documento', () => {
  test('ofrece los 26 idiomas, cada uno en el suyo, y sin idioma propio marca el de la interfaz', () => {
    montar();
    const sel = selectorIdioma();
    expect(sel.options.length).toBe(26);
    expect(sel.value).toBe('es');
    const de = [...sel.options].find((o) => o.value === 'de')!;
    expect(de.textContent).toBe('Deutsch');
    expect(de.lang).toBe('de');
  });

  test('marca el que guarda el documento', () => {
    montar('document', { ...PAGINA_POR_DEFECTO, lang: 'fr' });
    expect(selectorIdioma().value).toBe('fr');
  });

  test('elegir otro lo guarda en la capa de página del folio y avisa al autosave', () => {
    montar();
    let entradas = 0;
    folio.addEventListener('input', () => entradas++);
    const sel = selectorIdioma();
    act(() => {
      sel.value = 'de';
      sel.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(paginas.at(-1)?.lang).toBe('de');
    const capa = folio.firstElementChild!;
    expect(capa.hasAttribute('data-sytue-page')).toBe(true);
    expect(capa.getAttribute('data-lang')).toBe('de');
    expect(folio.querySelectorAll('[data-sytue-page]').length).toBe(1);
    expect(entradas).toBe(1);
  });

  test('en solo lectura no se puede cambiar', () => {
    montar('document', { ...PAGINA_POR_DEFECTO }, true);
    expect(selectorIdioma().disabled).toBe(true);
  });

  test('en Diapos no hay idioma de documento', () => {
    montar('presentation');
    expect(host!.querySelector('select')).toBeNull();
  });
});
