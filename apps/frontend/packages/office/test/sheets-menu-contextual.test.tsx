/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MENÚ CONTEXTUAL DE LA REJILLA (CALC-05): clic derecho en una celda o en una cabecera,
 * Mayús+F10 sobre la celda activa. Se prueba sobre el lienzo montado: qué variante sale,
 * qué selecciona el clic derecho, que las opciones despachan sus comandos (lo que se mira
 * es el modelo que sale por `onContentInput`), lo vetado por la protección y adónde vuelve
 * el foco al cerrar.
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

function montar(modelo: SheetModel, soloLectura = false): HTMLElement {
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
    soloLectura,
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
          </EditorCtx.Provider>
        </EditorI18nCtx.Provider>
      </ToastProvider>,
      el,
    );
  });
  return el;
}

const hoja = (cells: Record<string, string>, extra: object = {}): SheetModel =>
  ({ active: 0, sheets: [{ name: 'Hoja1', cells, ...extra }] }) as SheetModel;

const celda = (h: HTMLElement, ref: string): HTMLElement =>
  h.querySelector<HTMLElement>(`[data-ref="${ref}"]`)!;

function pulsar(el: HTMLElement, extra: MouseEventInit = {}): void {
  act(() => {
    el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, ...extra }));
    el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, ...extra }));
  });
}

function tecla(h: HTMLElement, key: string, mods: KeyboardEventInit = {}): KeyboardEvent {
  const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...mods });
  act(() => {
    h.querySelector<HTMLElement>('[role="grid"]')!.dispatchEvent(ev);
  });
  return ev;
}

/** Marca de `desde` a `hasta` como un clic y un Mayús+clic. */
function marcar(h: HTMLElement, desde: string, hasta: string): void {
  pulsar(celda(h, desde));
  pulsar(celda(h, hasta), { shiftKey: true });
}

/** Los efectos del menú corren «tras el pintado»; en happy-dom, con un setTimeout. */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 150));

const rejilla = (h: HTMLElement): HTMLElement => h.querySelector<HTMLElement>('[role="grid"]')!;
const menu = (): HTMLElement | null => document.querySelector<HTMLElement>('[role="menu"]');
const opcion = (texto: string): HTMLButtonElement =>
  [...document.querySelectorAll<HTMLButtonElement>('[role="menu"] [role="menuitem"]')].find(
    (b) => b.textContent === texto,
  )!;

async function clicDerecho(el: HTMLElement): Promise<MouseEvent> {
  const ev = new MouseEvent('contextmenu', {
    bubbles: true,
    cancelable: true,
    button: 2,
    clientX: 50,
    clientY: 60,
  });
  act(() => {
    el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 2 }));
    el.dispatchEvent(ev);
  });
  await tick();
  return ev;
}

/** Lo que enseña el cuadro de nombres (un `<input>` desde CALC-10). */
function cuadroNombres(h: HTMLElement): string {
  return h.querySelector<HTMLInputElement>('input[aria-label="Cuadro de nombres"]')?.value ?? '';
}

async function elegir(texto: string): Promise<void> {
  act(() => opcion(texto).click());
  await tick();
}

test('clic derecho en una celda: selecciona la celda, abre el menú de celdas y no el del navegador', async () => {
  const h = montar(hoja({ A1: 'a', A2: 'b' }));
  const ev = await clicDerecho(celda(h, 'A2'));
  expect(ev.defaultPrevented).toBe(true);
  expect(menu()?.getAttribute('aria-label')).toBe('Opciones de la celda');
  // El cuadro de nombre dice A2: la celda pulsada quedó seleccionada.
  expect(cuadroNombres(h)).toBe('A2');
  expect(opcion('Insertar fila encima')).toBeTruthy();
  expect(opcion('Ordenar de menor a mayor')).toBeTruthy();
  await elegir('Insertar fila encima');
  expect(menu()).toBeNull();
  // La fila nueva entra encima de la 2: «b» baja a la 3.
  expect(guardado!.sheets[0]!.cells['A3']).toBe('b');
  expect(guardado!.sheets[0]!.cells['A2']).toBeUndefined();
  // El foco, de vuelta en la rejilla para seguir con las flechas.
  expect(document.activeElement).toBe(rejilla(h));
});

test('clic derecho dentro del rango lo conserva: «Borrar contenido» vacía el rango entero', async () => {
  const h = montar(hoja({ A1: '1', B1: '2', A2: '3', B2: '4', C3: 'fuera' }));
  marcar(h, 'A1', 'B2');
  await clicDerecho(celda(h, 'B2'));
  await elegir('Borrar contenido');
  expect(guardado!.sheets[0]!.cells).toEqual({ C3: 'fuera' });
});

test('cabecera de columna: su menú, con el ancho y sin las opciones de filas', async () => {
  const h = montar(hoja({ B1: 'x' }));
  await clicDerecho(h.querySelector<HTMLElement>('[data-colhead="1"]')!);
  expect(menu()?.getAttribute('aria-label')).toBe('Opciones de la columna');
  expect(opcion('Ancho de columna…')).toBeTruthy();
  expect(opcion('Insertar fila encima')).toBeUndefined();
  // Seleccionó la columna B entera.
  expect(cuadroNombres(h)).toMatch(/^B1:B\d+$/);
  await elegir('Insertar columna a la izquierda');
  expect(guardado!.sheets[0]!.cells['C1']).toBe('x');
});

test('cabecera de fila: su menú, con el alto y sin ordenar', async () => {
  const h = montar(hoja({}));
  await clicDerecho(h.querySelector<HTMLElement>('[data-rowhead="2"]')!);
  expect(menu()?.getAttribute('aria-label')).toBe('Opciones de la fila');
  expect(opcion('Alto de fila…')).toBeTruthy();
  expect(opcion('Ordenar de menor a mayor')).toBeUndefined();
});

test('Mayús+F10 abre el menú de la celda activa y Escape lo cierra con el foco en la rejilla', async () => {
  const h = montar(hoja({ A1: 'a' }));
  pulsar(celda(h, 'A1'));
  const ev = new KeyboardEvent('keydown', {
    key: 'F10',
    shiftKey: true,
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    rejilla(h).dispatchEvent(ev);
  });
  await tick();
  expect(ev.defaultPrevented).toBe(true);
  expect(menu()?.getAttribute('aria-label')).toBe('Opciones de la celda');
  expect(document.activeElement?.textContent).toBe('Cortar');
  act(() => {
    document.activeElement!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    );
  });
  await tick();
  expect(menu()).toBeNull();
  expect(document.activeElement).toBe(rejilla(h));
});

test('con columnas enteras marcadas, el teclado abre el menú de la cabecera', async () => {
  const h = montar(hoja({}));
  pulsar(celda(h, 'B2'));
  tecla(h, ' ', { ctrlKey: true }); // Ctrl+Espacio: la columna entera
  tecla(h, 'ContextMenu');
  await tick();
  expect(menu()?.getAttribute('aria-label')).toBe('Opciones de la columna');
});

test('hoja protegida: lo vetado sale deshabilitado y con su motivo; copiar sigue', async () => {
  const h = montar(hoja({ A1: 'a' }, { protect: {} }));
  await clicDerecho(celda(h, 'A1'));
  const insertar = opcion('Insertar fila encima');
  expect(insertar.disabled).toBe(true);
  expect(insertar.getAttribute('aria-label')).toBe(
    'Insertar fila encima (no disponible: Hoja protegida)',
  );
  expect(opcion('Pegar').disabled).toBe(true);
  // Las celdas están bloqueadas por defecto: tampoco se corta ni se borra.
  expect(opcion('Borrar contenido').disabled).toBe(true);
  expect(opcion('Copiar').disabled).toBe(false);
  // El motivo también se ve, arriba del menú.
  expect(menu()?.textContent).toContain('Hoja protegida');
});

test('solo lectura: todo apagado menos copiar, con el motivo', async () => {
  const h = montar(hoja({ A1: 'a' }), true);
  await clicDerecho(celda(h, 'A1'));
  expect(opcion('Eliminar filas').getAttribute('aria-label')).toBe(
    'Eliminar filas (no disponible: Solo lectura)',
  );
  expect(opcion('Congelar hasta aquí').disabled).toBe(true);
  expect(opcion('Copiar').disabled).toBe(false);
});
