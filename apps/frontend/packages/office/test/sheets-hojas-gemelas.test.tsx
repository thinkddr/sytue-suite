/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * HOJAS GEMELAS: dos hojas que solo difieren en mayúsculas («Ventas» y «VENTAS»), que
 * Excel y LibreOffice no dejan tener y el servidor ya no deja crear. En la barra de
 * hojas, renombrar a una gemela se avisa bajo el campo y no se aplica, y añadir o
 * duplicar eligen el siguiente nombre libre. Se prueba sobre el lienzo montado con los
 * diálogos del editor, y lo que se mira es el modelo que sale por `onContentInput`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
// El lienzo pregunta por la sesión (vistas de filtro personales); aquí no hay servidor.
globalThis.fetch = (() =>
  Promise.resolve(new Response('{}', { status: 401 }))) as unknown as typeof fetch;

import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { DialogosDelEditor, ToastProvider } from '@sytue/ui';
import { docsT, editorT } from '../src/i18n';
import { EditorCtx, EditorI18nCtx } from '../src/editor/context';
import { SheetCanvas } from '../src/editor/sheets/SheetCanvas';
import type { EditorContext, SheetModel } from '../src/editor/types';

const nada = (): void => undefined;
const nadaAsync = (): Promise<void> => Promise.resolve();
const t = docsT('es');

let host: HTMLElement | null = null;
let guardado: SheetModel | null = null;

afterEach(() => {
  if (host) render(null, host);
  host?.remove();
  host = null;
  guardado = null;
});

function montar(nombres: string[]): HTMLElement {
  const modelo = {
    active: 0,
    sheets: nombres.map((name) => ({ name, cells: {} })),
  } as SheetModel;
  const ctx = {
    doc: { id: 'h', kind: 'spreadsheet', title: 'Hoja' },
    kind: 'spreadsheet',
    locale: 'es',
    t,
    aiEnabled: false,
    title: 'Hoja',
    setTitle: nada,
    save: 'saved',
    connection: 'online',
    content: JSON.stringify(modelo),
    contentEpoch: 1,
    onContentInput: (json: string) => {
      guardado = JSON.parse(json) as SheetModel;
    },
    exec: nada,
    ui: { panel: null, settingsOpen: false, ruler: true, zoom: 100, formFill: false, track: false },
    setUi: nada,
    pageSetup: { margin: 96, landscape: false },
    setPageSetup: nada,
    stats: { words: 0, pages: 1, page: 1 },
    reportStats: nada,
    peers: [],
    peerCursors: [],
    sendCursor: nada,
    comments: [],
    addComment: nadaAsync,
    resolveComment: nadaAsync,
    removeComment: nadaAsync,
    versions: [],
    loadVersions: nadaAsync,
    createVersion: nadaAsync,
    restoreVersion: nadaAsync,
    applyTheme: nadaAsync,
    exportAs: nadaAsync,
    saveNow: nada,
    sourceRef: '',
    esPropietario: false,
    cursorDots: { aPuntos: () => null, aOffsets: () => null },
    soloLectura: false,
    savingToDrive: false,
    saveToDrive: nadaAsync,
  } as unknown as EditorContext;
  const el = document.createElement('div');
  host = el;
  document.body.appendChild(el);
  act(() => {
    render(
      <ToastProvider>
        <DialogosDelEditor />
        <EditorI18nCtx.Provider value={t}>
          <EditorCtx.Provider value={ctx}>
            <SheetCanvas ctx={ctx} />
          </EditorCtx.Provider>
        </EditorI18nCtx.Provider>
      </ToastProvider>,
      el,
    );
  });
  return el;
}

const hojas = (): string[] => guardado?.sheets.map((s) => s.name) ?? [];

/** La pestaña de la hoja (el botón con su nombre, no el asa de arrastre). */
const pestana = (h: HTMLElement, nombre: string): HTMLButtonElement =>
  h.querySelector<HTMLButtonElement>(
    `button[aria-keyshortcuts="F2 Shift+F10"][title="${t('sheet.tab.hint', { name: nombre })}"]`,
  )!;

const boton = (h: HTMLElement, rotulo: string): HTMLButtonElement =>
  h.querySelector<HTMLButtonElement>(`button[aria-label="${rotulo}"]`)!;

/** Pulsa F2 en la pestaña y espera a que el diálogo de renombrar esté abierto. */
async function renombrar(h: HTMLElement, nombre: string): Promise<HTMLInputElement> {
  await act(async () => {
    pestana(h, nombre).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'F2', bubbles: true, cancelable: true }),
    );
    await Promise.resolve();
  });
  const campo = document.querySelector<HTMLInputElement>('#sytue-prompt');
  if (!campo) throw new Error('no se ha abierto el diálogo de renombrar');
  return campo;
}

function escribir(campo: HTMLInputElement, texto: string): void {
  act(() => {
    campo.value = texto;
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

/** El botón que acepta: el último del pie del diálogo (Cancelar va antes). */
const aceptar = (): HTMLButtonElement => {
  const bs = document.querySelectorAll<HTMLButtonElement>(
    '[role="dialog"] button, [role="alertdialog"] button',
  );
  return bs[bs.length - 1]!;
};

// VISTO EN ROJO sin la regla: «Resumen» pasaba a «VENTAS» al pulsar Intro.
test('renombrar a una gemela se avisa bajo el campo y no se aplica', async () => {
  const h = montar(['Ventas', 'Resumen']);
  const campo = await renombrar(h, 'Resumen');
  escribir(campo, 'VENTAS');
  // Los diálogos del lienzo van en el idioma del documento (`editorT`), como su pregunta.
  const aviso = editorT()('dialog.sheet.nameTaken', { name: 'Ventas' });
  expect(document.getElementById('sytue-prompt-error')?.textContent).toContain(aviso);
  expect(campo.getAttribute('aria-invalid')).toBe('true');
  expect(campo.getAttribute('aria-describedby')).toBe('sytue-prompt-error');
  expect(aceptar().disabled).toBe(true);
  act(() => {
    campo.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  });
  expect(document.querySelector('#sytue-prompt')).not.toBeNull();
  expect(guardado).toBeNull();

  // Con otro nombre el aviso se va y se renombra.
  escribir(campo, 'Ventas 2026');
  expect(document.getElementById('sytue-prompt-error')).toBeNull();
  expect(aceptar().disabled).toBe(false);
  await act(async () => {
    campo.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    await Promise.resolve();
  });
  expect(hojas()).toEqual(['Ventas', 'Ventas 2026']);
});

test('cambiar solo las mayúsculas de la propia hoja sí vale', async () => {
  const h = montar(['ventas', 'Resumen']);
  const campo = await renombrar(h, 'ventas');
  escribir(campo, 'Ventas');
  expect(document.getElementById('sytue-prompt-error')).toBeNull();
  await act(async () => {
    campo.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    await Promise.resolve();
  });
  expect(hojas()).toEqual(['Ventas', 'Resumen']);
});

// VISTO EN ROJO sin la regla: la hoja nueva se llamaba «Hoja 3», gemela de «hoja 3».
test('añadir una hoja elige el siguiente nombre libre salvo mayúsculas', () => {
  const h = montar(['Hoja 1', 'hoja 3']);
  act(() => boton(h, t('sheet.add')).click());
  expect(hojas()).toEqual(['Hoja 1', 'hoja 3', 'Hoja 4']);
});

// VISTO EN ROJO sin la regla: la copia se llamaba «Datos (2)», gemela de «DATOS (2)».
test('duplicar una hoja elige el siguiente nombre libre salvo mayúsculas', () => {
  const h = montar(['Datos', 'DATOS (2)']);
  act(() => boton(h, t('sheet.duplicate')).click());
  expect(hojas()).toEqual(['Datos', 'Datos (3)', 'DATOS (2)']);
});
