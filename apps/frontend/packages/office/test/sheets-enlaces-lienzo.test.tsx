/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los enlaces dentro de la rejilla de Calcus: la celda con HYPERLINK o con una URL se pinta
 * subrayada, Ctrl/⌘+clic y Alt+Intro la abren en otra pestaña con `noopener`, y la celda
 * activa enseña un chip que es un <a> de verdad. Un `javascript:` no se pinta ni se abre.
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

const modelo: SheetModel = {
  active: 0,
  sheets: [
    {
      name: 'Hoja1',
      cells: {
        A1: '=HYPERLINK("https://ejemplo.org/precios";"Precios")',
        A2: 'mailto:ana@ejemplo.org',
        A3: '=HYPERLINK("javascript:alert(1)";"Trampa")',
        A4: 'Texto normal',
      },
    },
  ],
} as SheetModel;

let host: HTMLElement | null = null;
const abiertas: unknown[][] = [];
const abrirOriginal = window.open;

afterEach(() => {
  if (host) render(null, host);
  host?.remove();
  host = null;
  abiertas.length = 0;
  window.open = abrirOriginal;
});

function montar(): HTMLElement {
  window.open = ((...a: unknown[]) => {
    abiertas.push(a);
    return null;
  }) as typeof window.open;
  const t = docsT('es');
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

const celda = (h: HTMLElement, ref: string): HTMLElement =>
  h.querySelector<HTMLElement>(`[data-ref="${ref}"]`)!;

function pulsar(el: HTMLElement, extra: MouseEventInit = {}): void {
  act(() => {
    el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, ...extra }));
    el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, ...extra }));
  });
}

test('las celdas con enlace se pintan subrayadas y dicen cómo abrirse', () => {
  const h = montar();
  const a1 = celda(h, 'A1');
  expect(a1.textContent).toBe('Precios');
  expect(a1.style.textDecorationLine).toBe('underline');
  expect(a1.title).toContain('https://ejemplo.org/precios');
  expect(celda(h, 'A2').style.textDecorationLine).toBe('underline');
  // Ni el esquema vetado ni el texto normal se vuelven enlace.
  expect(celda(h, 'A3').style.textDecorationLine).toBe('');
  expect(celda(h, 'A3').title).toBe('');
  expect(celda(h, 'A4').style.textDecorationLine).toBe('');
});

test('la celda activa con enlace enseña un chip <a> seguro; sin enlace, no hay chip', () => {
  const h = montar();
  const chip = h.querySelector<HTMLAnchorElement>('a[data-enlace-celda]');
  expect(chip).not.toBeNull(); // A1 es la celda activa al abrir
  expect(chip!.getAttribute('href')).toBe('https://ejemplo.org/precios');
  expect(chip!.getAttribute('target')).toBe('_blank');
  expect(chip!.getAttribute('rel')).toBe('noopener noreferrer');
  expect(chip!.textContent).toBe('ejemplo.org');
  expect(chip!.getAttribute('aria-label')).toBe('Abrir enlace https://ejemplo.org/precios');

  pulsar(celda(h, 'A4'));
  expect(h.querySelector('a[data-enlace-celda]')).toBeNull();
  pulsar(celda(h, 'A3'));
  expect(h.querySelector('a[data-enlace-celda]')).toBeNull();
  pulsar(celda(h, 'A2'));
  expect(h.querySelector('a[data-enlace-celda]')?.getAttribute('href')).toBe(
    'mailto:ana@ejemplo.org',
  );
});

test('el clic solo selecciona; Ctrl/⌘+clic abre en otra pestaña con noopener', () => {
  const h = montar();
  pulsar(celda(h, 'A2'));
  expect(abiertas).toEqual([]);
  pulsar(celda(h, 'A1'), { ctrlKey: true });
  pulsar(celda(h, 'A2'), { metaKey: true });
  pulsar(celda(h, 'A3'), { ctrlKey: true }); // javascript: no se abre nunca
  pulsar(celda(h, 'A4'), { ctrlKey: true });
  expect(abiertas).toEqual([
    ['https://ejemplo.org/precios', '_blank', 'noopener,noreferrer'],
    ['mailto:ana@ejemplo.org', '_blank', 'noopener,noreferrer'],
  ]);
});

test('Alt+Intro abre el enlace de la celda activa en vez de editarla', () => {
  const h = montar();
  const rejilla = h.querySelector<HTMLElement>('[role="grid"]')!;
  act(() => {
    rejilla.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', altKey: true, bubbles: true, cancelable: true }),
    );
  });
  expect(abiertas).toEqual([['https://ejemplo.org/precios', '_blank', 'noopener,noreferrer']]);
  expect(celda(h, 'A1').textContent).toBe('Precios'); // sigue sin editar
});

test('escribiendo en la celda no hay chip del enlace (como en la app nativa)', () => {
  const h = montar();
  expect(h.querySelector('a[data-enlace-celda]')).not.toBeNull();
  const rejilla = h.querySelector<HTMLElement>('[role="grid"]')!;
  act(() => {
    rejilla.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'F2', bubbles: true, cancelable: true }),
    );
  });
  expect(h.querySelector('a[data-enlace-celda]')).toBeNull();
});
