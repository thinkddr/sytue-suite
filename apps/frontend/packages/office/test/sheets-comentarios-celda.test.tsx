/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * COMENTARIOS ANCLADOS A UNA CELDA (sheets/anclaCelda.ts): «Nuevo comentario» en una hoja
 * cuelga el hilo de la celda activa, la rejilla marca la celda con un triángulo y el primer
 * comentario en su `title`, la marca sigue a la celda al insertar una fila encima y resolver
 * el hilo la quita. Se prueban el ancla pura y el lienzo montado.
 * Ejecutar: cd apps/frontend/packages/office && bun test test/sheets-comentarios-celda.test.tsx
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
// El lienzo pregunta por la sesión (vistas de filtro personales); aquí no hay servidor.
globalThis.fetch = (() =>
  Promise.resolve(new Response('{}', { status: 401 }))) as unknown as typeof fetch;

import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { ToastProvider } from '@sytue/ui';
import { docsT } from '../src/i18n';
import { EditorCtx, EditorI18nCtx } from '../src/editor/context';
import {
  SheetCanvas,
  readCommentCellRef,
  readSheetCellAnchor,
  revealCommentCell,
} from '../src/editor/sheets/SheetCanvas';
import { conEstructura } from '../src/editor/sheets/anclas';
import { CommentsPanel } from '../src/editor/panels/CommentsPanel';
import {
  anclaDeCelda,
  celdaDeAncla,
  leerAnclaCelda,
  marcasDeComentarios,
} from '../src/editor/sheets/anclaCelda';
import type { CommentItem, EditorContext, SheetData, SheetModel } from '../src/editor/types';

const t = docsT('es');
const nada = (): void => undefined;
const nadaAsync = (): Promise<void> => Promise.resolve();

const hoja = (extra: Partial<SheetData> = {}): SheetData => ({
  id: 'h1',
  name: 'Hoja1',
  cells: {},
  ...extra,
});

const comentario = (anchor: unknown, extra: Partial<CommentItem> = {}): CommentItem => ({
  id: `c${Math.random().toString(16).slice(2, 8)}`,
  document_id: 'd',
  author_sub: 'abcdef1234',
  author_name: 'Ana',
  anchor,
  body: 'Revisar este total',
  resolved: false,
  created_at: '2026-11-16T10:00:00Z',
  updated_at: '2026-11-16T10:00:00Z',
  ...extra,
});

/* ---------- El ancla, sin DOM ---------- */

test('el ancla de B4 lleva la hoja y la identidad de su fila y su columna', () => {
  const a = anclaDeCelda(hoja(), 1, 3);
  expect(a).toEqual({ hoja: 'h1', fila: 't3', col: 't1', text: 'B4' });
  expect(leerAnclaCelda(a)).toEqual(a);
  // Las de un folio o un dibujo no son de celda.
  expect(leerAnclaCelda({ mark: 'abc', text: 'hola' })).toBeNull();
  expect(leerAnclaCelda({ hoja: 'h1', fila: 'no vale', col: 't1' })).toBeNull();
  expect(leerAnclaCelda(null)).toBeNull();
  // Sin id de hoja no hay ancla (el lienzo siempre se lo da: conIdsDeHoja).
  expect(anclaDeCelda({}, 0, 0)).toBeNull();
});

test('insertar una fila encima lleva el comentario de B4 a B5; borrarla lo deja sin celda', () => {
  const s = hoja();
  const a = anclaDeCelda(s, 1, 3)!;
  const conFila: SheetData = { ...s, ...conEstructura(s, 'fila', 0, 1) };
  expect(celdaDeAncla({ sheets: [conFila] }, a)).toEqual({ hoja: 0, c: 1, r: 4, ref: 'B5' });
  const sinSuFila: SheetData = { ...conFila, ...conEstructura(conFila, 'fila', 4, -1) };
  expect(celdaDeAncla({ sheets: [sinSuFila] }, a)).toBeNull();
});

test('marcan las celdas los hilos abiertos de esa hoja; las respuestas y los resueltos no', () => {
  const m = { sheets: [hoja(), hoja({ id: 'h2', name: 'Hoja2' })] };
  const b4 = anclaDeCelda(m.sheets[0]!, 1, 3)!;
  const viejo = comentario(b4, { body: 'El primero', created_at: '2026-11-15T09:00:00Z' });
  const marcas = marcasDeComentarios(m, 0, [
    comentario(b4),
    viejo,
    comentario(b4, { parent_id: viejo.id }),
    comentario(anclaDeCelda(m.sheets[0]!, 0, 0), { resolved: true }),
    comentario(anclaDeCelda(m.sheets[1]!, 2, 2)),
    comentario({ mark: 'abc', text: 'B4' }),
  ]);
  expect([...marcas.keys()]).toEqual(['B4']);
  expect(marcas.get('B4')!.hilos).toBe(2);
  expect(marcas.get('B4')!.primero.body).toBe('El primero');
  expect([
    ...marcasDeComentarios(m, 1, [comentario(anclaDeCelda(m.sheets[1]!, 2, 2))]).keys(),
  ]).toEqual(['C3']);
});

/* ---------- El lienzo montado ---------- */

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) render(null, host);
  host?.remove();
  host = null;
});

function montar(modelo: SheetModel, comments: CommentItem[], conPanel = false): HTMLElement {
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
    onContentInput: nada,
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
    comments,
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
        <EditorI18nCtx.Provider value={t}>
          <EditorCtx.Provider value={ctx}>
            <SheetCanvas ctx={ctx} />
            {conPanel ? <CommentsPanel ctx={ctx} /> : null}
          </EditorCtx.Provider>
        </EditorI18nCtx.Provider>
      </ToastProvider>,
      el,
    );
  });
  return el;
}

const celda = (h: HTMLElement, ref: string): HTMLElement =>
  h.querySelector<HTMLElement>(`[data-ref="${ref}"]`)!;

function pulsar(el: HTMLElement): void {
  act(() => {
    el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true }));
    el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true }));
  });
}

test('la celda comentada lleva el triángulo y el primer comentario en su title', () => {
  const s = hoja({ cells: { B4: '120' } });
  const b4 = anclaDeCelda(s, 1, 3)!;
  const h = montar({ active: 0, sheets: [s] }, [comentario(b4)]);
  const c = celda(h, 'B4');
  expect(c.style.backgroundImage).toContain('linear-gradient');
  expect(c.getAttribute('title')).toBe('Ana: Revisar este total');
  expect(c.getAttribute('aria-description')).toBe('Ana: Revisar este total');
  expect(celda(h, 'B3').style.backgroundImage).toBe('');
  expect(celda(h, 'B3').getAttribute('title')).toBeNull();
});

test('resuelto, el hilo ya no marca la celda', () => {
  const s = hoja();
  const h = montar({ active: 0, sheets: [s] }, [
    comentario(anclaDeCelda(s, 1, 3), { resolved: true }),
  ]);
  expect(celda(h, 'B4').style.backgroundImage).toBe('');
  expect(celda(h, 'B4').getAttribute('title')).toBeNull();
});

test('con una fila insertada encima, la marca está en B5', () => {
  const s = hoja();
  const b4 = anclaDeCelda(s, 1, 3)!;
  const movida: SheetData = { ...s, ...conEstructura(s, 'fila', 0, 1) };
  const h = montar({ active: 0, sheets: [movida] }, [comentario(b4)]);
  expect(celda(h, 'B4').style.backgroundImage).toBe('');
  expect(celda(h, 'B5').style.backgroundImage).toContain('linear-gradient');
  expect(readCommentCellRef(b4)).toBe('B5');
});

test('el ancla nueva es la de la celda activa, y el panel lleva a la celda de otra hoja', () => {
  const h1 = hoja();
  const h2 = hoja({ id: 'h2', name: 'Hoja2' });
  const h = montar({ active: 0, sheets: [h1, h2] }, []);
  pulsar(celda(h, 'C2'));
  expect(readSheetCellAnchor()).toEqual({ hoja: 'h1', fila: 't1', col: 't2', text: 'C2' });
  const enOtra = anclaDeCelda(h2, 3, 6)!;
  expect(readCommentCellRef(enOtra)).toBe('Hoja2!D7');
  act(() => {
    expect(revealCommentCell(enOtra)).toBe(true);
  });
  expect(readSheetCellAnchor()).toEqual({ hoja: 'h2', fila: 't6', col: 't3', text: 'D7' });
  // Una celda que ya no existe no lleva a ningún sitio.
  expect(revealCommentCell({ hoja: 'nadie', fila: 't0', col: 't0', text: 'A1' })).toBe(false);
});

test('el hilo del panel dice su celda de hoy y lleva a ella', () => {
  const s = hoja();
  const b4 = anclaDeCelda(s, 1, 3)!;
  const movida: SheetData = { ...s, ...conEstructura(s, 'fila', 0, 1) };
  const h = montar({ active: 0, sheets: [movida] }, [comentario(b4)], true);
  const boton = [...h.querySelectorAll('button')].find((b) => b.textContent === 'Celda B5');
  expect(boton, 'el hilo no dice «Celda B5»').toBeDefined();
  expect(readSheetCellAnchor()?.text).toBe('A1');
  act(() => boton!.click());
  expect(readSheetCellAnchor()).toEqual({ ...b4, text: 'B5' });
});

test('al pasar por el hilo se enmarca su celda, y al salir deja de estarlo', () => {
  const s = hoja();
  const b4 = anclaDeCelda(s, 1, 3)!;
  const h = montar({ active: 0, sheets: [s] }, [comentario(b4)], true);
  const hilo = [...h.querySelectorAll('button')]
    .find((b) => b.textContent === 'Celda B4')!
    .closest('article')!;
  expect(celda(h, 'B4').style.boxShadow).toBe('');
  act(() => {
    hilo.dispatchEvent(new MouseEvent('mouseenter'));
  });
  expect(celda(h, 'B4').style.boxShadow).toContain('inset');
  // Enmarcar no mueve la selección.
  expect(readSheetCellAnchor()?.text).toBe('A1');
  act(() => {
    hilo.dispatchEvent(new MouseEvent('mouseleave'));
  });
  expect(celda(h, 'B4').style.boxShadow).toBe('');
});
