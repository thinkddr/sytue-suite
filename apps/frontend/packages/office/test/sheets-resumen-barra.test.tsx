/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL RESUMEN A LA CARTA Y LA BARRA FX DESPLEGABLE: el resumen del rango marcado enseña
 * solo la suma hasta que se elige otra cosa en su menú, y lo elegido se recuerda en el
 * dispositivo; la barra de fórmulas se despliega en tres líneas para leer una fórmula
 * larga, se recuerda igual, y Enter sigue escribiendo la celda.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
// El lienzo pregunta por la sesión (vistas de filtro personales); aquí no hay servidor.
globalThis.fetch = (() =>
  Promise.resolve(new Response('{}', { status: 401 }))) as unknown as typeof fetch;

import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { ToastProvider } from '@sytue/ui';
import { docsT } from '../src/i18n';
import { EditorCtx, EditorI18nCtx } from '../src/editor/context';
import { SheetCanvas } from '../src/editor/sheets/SheetCanvas';
import { selectionStats } from '../src/editor/sheets/format';
import { alternarMedida, leerMedidas, textoResumen } from '../src/editor/sheets/resumenBarra';
import type { EditorContext, SheetModel } from '../src/editor/types';

beforeEach(() => localStorage.clear());

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

/** Marca de `desde` a `hasta` como un clic y un Mayús+clic. */
function marcar(h: HTMLElement, desde: string, hasta: string): void {
  pulsar(celda(h, desde));
  pulsar(celda(h, hasta), { shiftKey: true });
}

const resumen = (h: HTMLElement): string => h.querySelector('output')?.textContent ?? '';

const boton = (h: HTMLElement, label: string): HTMLButtonElement =>
  h.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;

function clic(el: HTMLElement): void {
  act(() => {
    el.click();
  });
}

const casilla = (texto: string): HTMLButtonElement =>
  [...document.querySelectorAll<HTMLButtonElement>('[role="menuitemcheckbox"]')].find((b) =>
    b.textContent?.startsWith(texto),
  )!;

test('leerMedidas: lo guardado, en el orden de Excel; sin nada válido, la suma', () => {
  expect(leerMedidas(null)).toEqual(['sum']);
  expect(leerMedidas('')).toEqual(['sum']);
  expect(leerMedidas('basura,,x')).toEqual(['sum']);
  expect(leerMedidas('sum,min,min,average')).toEqual(['average', 'min', 'sum']);
});

test('alternarMedida pone y quita, pero nunca deja el resumen vacío', () => {
  expect(alternarMedida(['sum'], 'max')).toEqual(['max', 'sum']);
  expect(alternarMedida(['max', 'sum'], 'sum')).toEqual(['max']);
  expect(alternarMedida(['sum'], 'sum')).toEqual(['sum']);
});

test('textoResumen junta las medidas elegidas con «·»', () => {
  const s = selectionStats([4, -1, 'x', 7])!;
  expect(textoResumen(t, ['count', 'numbers', 'min', 'max'], s)).toBe(
    'Recuento: 4 · Recuento numérico: 3 · Mín.: -1 · Máx.: 7',
  );
});

test('el resumen enseña solo la suma hasta que se elige otra cosa, y se recuerda', () => {
  const modelo = hoja({ A1: '1', A2: '2', A3: '3', A4: 'x' });
  let h = montar(modelo);
  marcar(h, 'A1', 'A4');
  expect(resumen(h)).toBe('Suma: 6');
  // El resumen es el disparador de su menú: cada medida con su cifra y su casilla.
  const disparador = h.querySelector<HTMLButtonElement>('button[title="Resumen de la selección"]')!;
  expect(disparador.getAttribute('aria-haspopup')).toBe('menu');
  clic(disparador);
  expect(casilla('Suma').getAttribute('aria-checked')).toBe('true');
  // La única marcada no se puede quitar (un resumen vacío no se podría volver a pulsar),
  // pero no se apaga: en gris no se leía. Pulsarla no la quita.
  expect(casilla('Suma').disabled).toBe(false);
  clic(casilla('Suma'));
  expect(resumen(h)).toBe('Suma: 6');
  expect(casilla('Suma').getAttribute('aria-checked')).toBe('true');
  expect(casilla('Mín.').getAttribute('aria-checked')).toBe('false');
  expect(casilla('Recuento numérico').textContent).toBe('Recuento numérico: 3');
  clic(casilla('Máx.'));
  clic(casilla('Recuento:'));
  expect(resumen(h)).toBe('Recuento: 4 · Máx.: 3 · Suma: 6');
  expect(casilla('Máx.').getAttribute('aria-checked')).toBe('true');
  expect(casilla('Suma').disabled).toBe(false);
  clic(casilla('Suma'));
  expect(resumen(h)).toBe('Recuento: 4 · Máx.: 3');
  // Es preferencia del dispositivo: otro libro abierto después la conserva.
  render(null, host!);
  h = montar(modelo);
  marcar(h, 'A1', 'A3');
  expect(resumen(h)).toBe('Recuento: 3 · Máx.: 3');
});

test('la barra fx se despliega en un textarea de tres líneas, se recuerda y Enter escribe', () => {
  const largo = '=SUMA(A1;A2)+' + 'A3*2+'.repeat(30) + '1';
  let h = montar(hoja({ A1: '1', B1: largo }));
  pulsar(celda(h, 'B1'));
  const abrir = boton(h, 'Desplegar la barra de fórmulas');
  expect(abrir.getAttribute('aria-expanded')).toBe('false');
  expect(h.querySelector('textarea')).toBeNull();
  clic(abrir);
  const area = h.querySelector<HTMLTextAreaElement>('textarea[aria-label="Barra de fórmulas"]')!;
  expect(area).not.toBeNull();
  expect(area.getAttribute('rows')).toBe('3');
  expect(area.value).toBe(largo);
  const recoger = boton(h, 'Recoger la barra de fórmulas');
  expect(recoger.getAttribute('aria-expanded')).toBe('true');
  expect(recoger.getAttribute('aria-controls')).toBe(area.id);
  // Se recuerda en el dispositivo.
  render(null, host!);
  h = montar(hoja({ A1: '1' }));
  const fx = h.querySelector<HTMLTextAreaElement>('textarea[aria-label="Barra de fórmulas"]')!;
  expect(fx).not.toBeNull();
  // Lo pegado con saltos de línea entra en una línea, como en el `<input>`.
  act(() => {
    fx.focus();
    fx.value = 'ho\r\nla\n';
    fx.setSelectionRange(4, 4);
    fx.dispatchEvent(new Event('input', { bubbles: true }));
  });
  expect(fx.value).toBe('hola');
  expect(fx.selectionStart).toBe(2);
  // Enter escribe la celda y no mete un salto de línea.
  act(() => {
    fx.value = 'hola';
    fx.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const ev = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });
  act(() => {
    fx.dispatchEvent(ev);
  });
  expect(ev.defaultPrevented).toBe(true);
  expect(guardado!.sheets[0]!.cells['A1']).toBe('hola');
  // Recogerla vuelve al `<input>` de una línea.
  clic(boton(h, 'Recoger la barra de fórmulas'));
  expect(h.querySelector('textarea')).toBeNull();
  expect(h.querySelector('input[aria-label="Barra de fórmulas"]')).not.toBeNull();
});
