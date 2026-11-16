/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS ATAJOS DE HOJA QUE FALTABAN: Ctrl+D y Ctrl+R rellenan, Ctrl+; escribe la fecha de
 * hoy, Mayús+Espacio y Ctrl+Espacio marcan filas y columnas enteras, y Ctrl+/ (o el botón
 * del teclado de la barra de hojas) abre la hoja de atajos. Se prueban sobre el lienzo
 * montado, con la rejilla enfocada como la deja un clic, y lo que se mira es el modelo
 * que sale por `onContentInput`, que es lo que se guarda.
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
import { gruposDeAtajos } from '../src/editor/atajosAyuda';
import {
  GRUPOS_HOJA,
  atajoDeHoja,
  atajosPropiosDeHoja,
  gruposDeAtajosHoja,
  rellenoDeAtajo,
  serialDeHoy,
} from '../src/editor/sheets/atajosHoja';
import type { EditorContext, SheetModel } from '../src/editor/types';

const nada = (): void => undefined;
const nadaAsync = (): Promise<void> => Promise.resolve();
const t = docsT('es');

let host: HTMLElement | null = null;
let guardado: SheetModel | null = null;
/** Lo que el lienzo pidió a la interfaz del editor (la hoja de atajos va por `setUi`). */
let pedidoUi: Record<string, unknown>[] = [];

afterEach(() => {
  if (host) render(null, host);
  host?.remove();
  host = null;
  guardado = null;
  pedidoUi = [];
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
    setUi: (patch: Record<string, unknown>) => {
      pedidoUi.push(patch);
    },
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

/** El cuadro de nombres de la barra de fórmulas. */
const cuadro = (h: HTMLElement): HTMLInputElement =>
  h.querySelector<HTMLInputElement>(`input[aria-label="${t('sheet.nameBox')}"]`)!;

/** Escribe en el cuadro de nombres y pulsa `key` (Intro por defecto). */
function escribirEnCuadro(h: HTMLElement, texto: string, key = 'Enter'): KeyboardEvent {
  const c = cuadro(h);
  act(() => c.focus());
  act(() => {
    c.value = texto;
    c.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
  act(() => {
    c.dispatchEvent(ev);
  });
  return ev;
}

/** Marca de `desde` a `hasta` como un clic y un Mayús+clic. */
function marcar(h: HTMLElement, desde: string, hasta: string): void {
  pulsar(celda(h, desde));
  pulsar(celda(h, hasta), { shiftKey: true });
}

test('Ctrl+D copia la primera fila del rango sobre las demás, ajustando las referencias', () => {
  const h = montar(hoja({ A1: '7', B1: '=A1*2', A2: 'x' }));
  marcar(h, 'A1', 'B3');
  const ev = tecla(h, 'd', { ctrlKey: true });
  expect(ev.defaultPrevented).toBe(true); // sin esto el navegador guardaba un marcador
  const c = guardado!.sheets[0]!.cells;
  expect(c['A2']).toBe('7');
  expect(c['A3']).toBe('7');
  expect(c['B2']).toBe('=A2*2');
  expect(c['B3']).toBe('=A3*2');
});

test('Ctrl+D con una sola fila copia la de encima; en la fila 1 no hace nada', () => {
  const h = montar(hoja({ A1: 'arriba', B1: '=A1' }));
  marcar(h, 'A2', 'B2');
  tecla(h, 'D', { ctrlKey: true, shiftKey: false });
  expect(guardado!.sheets[0]!.cells['A2']).toBe('arriba');
  expect(guardado!.sheets[0]!.cells['B2']).toBe('=A2');
  guardado = null;
  pulsar(celda(h, 'A1'));
  tecla(h, 'd', { metaKey: true });
  expect(guardado).toBeNull();
});

test('Ctrl+R copia la primera columna hacia la derecha', () => {
  const h = montar(hoja({ A1: '=B9', A2: '3' }));
  marcar(h, 'A1', 'C2');
  tecla(h, 'r', { ctrlKey: true });
  const c = guardado!.sheets[0]!.cells;
  expect(c['B1']).toBe('=C9');
  expect(c['C1']).toBe('=D9');
  expect(c['C2']).toBe('3');
});

test('Ctrl+; escribe la fecha de hoy como valor, con formato de fecha', () => {
  const h = montar(hoja({ B2: '<b>viejo</b>' }));
  pulsar(celda(h, 'B2'));
  tecla(h, ';', { ctrlKey: true });
  const s = guardado!.sheets[0]!;
  expect(s.cells['B2']).toBe(`<b>${serialDeHoy(new Date())}</b>`); // la negrita se queda
  expect(s.formats?.['B2']).toEqual({ id: 'date' });
});

test('en solo lectura, rellenar y la fecha no escriben nada', () => {
  const h = montar(hoja({ A1: '1' }), true);
  marcar(h, 'A1', 'A3');
  tecla(h, 'd', { ctrlKey: true });
  tecla(h, ';', { ctrlKey: true });
  expect(guardado).toBeNull();
});

test('Mayús+Espacio marca la fila entera y Ctrl+Espacio la columna, sin empezar a escribir', () => {
  const h = montar(hoja({ A1: '1', B2: 'b' }));
  pulsar(celda(h, 'B2'));
  const ev = tecla(h, ' ', { shiftKey: true });
  expect(ev.defaultPrevented).toBe(true);
  // La selección se ve en el cuadro de nombre: «A2:…2».
  expect(cuadro(h).value).toMatch(/^A2:[A-Z]+2$/);
  expect(celda(h, 'B2').textContent).toBe('b'); // no se abrió la celda con un espacio
  pulsar(celda(h, 'B2'));
  tecla(h, ' ', { ctrlKey: true });
  expect(cuadro(h).value).toMatch(/^B1:B\d+$/);
});

test('Ctrl+/ abre la hoja de atajos del editor, y el botón de la barra dice su atajo', () => {
  const h = montar(hoja({}));
  pulsar(celda(h, 'A1'));
  tecla(h, '/', { ctrlKey: true, shiftKey: true }); // en un teclado español, «/» es Mayús+7
  // Una sola hoja para todo el editor (`atajosAyuda.ts`): la rejilla la pide, no la monta.
  expect(pedidoUi).toEqual([{ atajos: true }]);
  const boton = h.querySelector<HTMLButtonElement>('button[aria-label="Atajos de teclado"]')!;
  expect(boton.getAttribute('aria-keyshortcuts')).toContain('Control+/');
  act(() => boton.click());
  expect(pedidoUi).toEqual([{ atajos: true }, { atajos: true }]);
});

test('la hoja del editor en Calcus lleva los de la rejilla sin repetir los del registro', () => {
  const grupos = gruposDeAtajos('spreadsheet', t, atajosPropiosDeHoja(t, false), false);
  const filas = grupos.flatMap((g) => g.atajos);
  const descripciones = filas.map((f) => f.descripcion);
  expect(descripciones).toContain(t('sheet.shortcuts.fillDown'));
  expect(descripciones).toContain(t('sheet.shortcuts.column'));
  // Ninguna combinación sale dos veces: copiar, pegar o la propia Ctrl+/ van una vez.
  const teclas = filas.map((f) => f.teclas.join(' '));
  expect(new Set(teclas).size).toBe(teclas.length);
  expect(teclas.filter((k) => k === 'Ctrl+/')).toHaveLength(1);
});

test('rellenoDeAtajo: origen y destino con el criterio de Excel', () => {
  expect(rellenoDeAtajo({ c1: 1, c2: 2, r1: 3, r2: 5 }, 'abajo')).toEqual({
    origen: { c1: 1, c2: 2, r1: 3, r2: 3 },
    destino: { c1: 1, c2: 2, r1: 4, r2: 5 },
  });
  expect(rellenoDeAtajo({ c1: 1, c2: 1, r1: 3, r2: 3 }, 'abajo')).toEqual({
    origen: { c1: 1, c2: 1, r1: 2, r2: 2 },
    destino: { c1: 1, c2: 1, r1: 3, r2: 3 },
  });
  expect(rellenoDeAtajo({ c1: 0, c2: 0, r1: 0, r2: 4 }, 'derecha')).toBeNull();
  expect(rellenoDeAtajo({ c1: 2, c2: 4, r1: 0, r2: 0 }, 'derecha')).toEqual({
    origen: { c1: 2, c2: 2, r1: 0, r2: 0 },
    destino: { c1: 3, c2: 4, r1: 0, r2: 0 },
  });
});

test('serialDeHoy es el número de serie del día local (sistema 1900)', () => {
  expect(serialDeHoy(new Date(2026, 10, 13, 23, 59))).toBe(46339);
  expect(serialDeHoy(new Date(1900, 2, 1))).toBe(61); // tras el día fantasma
});

test('atajoDeHoja: cada pulsación y las que no son', () => {
  const k = (key: string, m: Partial<Record<'ctrl' | 'meta' | 'alt' | 'shift', boolean>> = {}) =>
    atajoDeHoja({
      key,
      ctrlKey: !!m.ctrl,
      metaKey: !!m.meta,
      altKey: !!m.alt,
      shiftKey: !!m.shift,
    });
  expect(k('d', { ctrl: true })).toBe('rellenarAbajo');
  expect(k('r', { meta: true })).toBe('rellenarDerecha');
  expect(k(';', { ctrl: true, shift: true })).toBe('hoy');
  expect(k('/', { ctrl: true })).toBe('ayuda');
  expect(k(' ', { shift: true })).toBe('fila');
  expect(k(' ', { ctrl: true })).toBe('columna');
  expect(k(' ')).toBeNull();
  expect(k('d')).toBeNull();
  expect(k('d', { ctrl: true, alt: true })).toBeNull(); // AltGr
  expect(k('D', { ctrl: true, shift: true })).toBeNull();
  expect(k('g', { ctrl: true })).toBe('irA');
  expect(k('G', { meta: true })).toBe('irA');
  expect(k('F5')).toBe('irA');
  expect(k('F5', { ctrl: true })).toBeNull(); // Ctrl+F5 sigue recargando sin caché
  expect(k('g')).toBeNull();
});

test('Ctrl+G y F5 llevan al cuadro de nombres sin que el navegador busque ni recargue', () => {
  const h = montar(hoja({}));
  pulsar(celda(h, 'B2'));
  const ev = tecla(h, 'g', { ctrlKey: true });
  expect(ev.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(cuadro(h));
  pulsar(celda(h, 'C3'));
  h.querySelector<HTMLElement>('[role="grid"]')!.focus();
  const f5 = tecla(h, 'F5');
  expect(f5.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(cuadro(h));
});

test('el cuadro de nombres salta a una celda o a un rango y devuelve el foco a la hoja', () => {
  const h = montar(hoja({ A1: '1' }));
  pulsar(celda(h, 'A1'));
  expect(cuadro(h).value).toBe('A1');
  escribirEnCuadro(h, 'c40');
  expect(cuadro(h).value).toBe('C40');
  expect(document.activeElement).toBe(h.querySelector('[role="grid"]'));
  escribirEnCuadro(h, 'AA1000:AB1200');
  expect(cuadro(h).value).toBe('AA1000:AB1200');
  // Lo que no cabe en el cuadro se lee entero al pasar por encima.
  expect(cuadro(h).title).toBe('AA1000:AB1200');
});

test('el cuadro de nombres cambia de hoja y entiende los nombres definidos', () => {
  const modelo = {
    active: 0,
    sheets: [
      { name: 'Hoja1', cells: {} },
      { name: 'Ventas 2026', cells: {} },
    ],
    names: [{ name: 'Cierre', ref: "'Ventas 2026'!$B$2:$B$13" }],
  } as unknown as SheetModel;
  const h = montar(modelo);
  escribirEnCuadro(h, "'Ventas 2026'!D4");
  expect(guardado!.active).toBe(1);
  expect(cuadro(h).value).toBe('D4');
  escribirEnCuadro(h, 'hoja1!B2');
  expect(guardado!.active).toBe(0);
  escribirEnCuadro(h, 'cierre');
  expect(guardado!.active).toBe(1);
  expect(cuadro(h).value).toBe('B2:B13');
});

test('lo que no lleva a ninguna parte avisa y no mueve la selección; Escape vuelve sin moverse', () => {
  const h = montar(hoja({}));
  pulsar(celda(h, 'B2'));
  escribirEnCuadro(h, 'NoHay!A1');
  expect(document.body.textContent).toContain(t('sheet.goTo.error'));
  expect(document.body.textContent).toContain(t('sheet.goTo.hoja', { texto: 'NoHay!A1' }));
  // Sigue escrito y con el foco, para corregirlo.
  expect(document.activeElement).toBe(cuadro(h));
  expect(cuadro(h).value).toBe('NoHay!A1');
  escribirEnCuadro(h, 'Z9', 'Escape');
  expect(document.activeElement).toBe(h.querySelector('[role="grid"]'));
  expect(cuadro(h).value).toBe('B2');
});

test('la hoja de atajos está entera en el idioma de quien lee', () => {
  const grupos = gruposDeAtajosHoja(t, false);
  expect(grupos.map((g) => g.titulo)).toEqual(['Moverse', 'Seleccionar', 'Editar']);
  const filas = grupos.flatMap((g) => g.atajos);
  expect(filas.length).toBe(GRUPOS_HOJA.reduce((n, g) => n + g.filas.length, 0));
  const teclas = filas.flatMap((f) => f.teclas);
  expect(teclas).toContain('Mayús+Espacio');
  expect(teclas).toContain('Ctrl+Mayús+V');
  expect(teclas).toContain('Ctrl+D');
  expect(teclas).toContain('Ctrl+Espacio');
  // El menú contextual del teclado, que la rejilla y la pestaña atienden desde CALC-05.
  const menu = filas.find((f) => f.descripcion === 'Abrir el menú contextual');
  expect(menu?.teclas).toEqual(['Mayús+F10']);
  for (const f of filas) {
    expect(f.descripcion).not.toMatch(/^[a-z]+(\.[a-zA-Z]+)+$/); // ninguna clave sin traducir
    for (const k of f.teclas) expect(k).not.toContain('{');
  }
  // En el Mac, con sus símbolos.
  const mac = gruposDeAtajosHoja(t, true).flatMap((g) => g.atajos.flatMap((a) => a.teclas));
  expect(mac).toContain('⌘D');
  expect(mac).toContain('⇧Espacio');
  // La columna entera es ⌃Espacio también en el Mac: ⌘+Espacio lo coge Spotlight.
  expect(mac).toContain('⌃Espacio');
  expect(mac).toContain('⇧F10');
  expect(mac).not.toContain('⌘Espacio');
});

test('el ▾ del cuadro de nombres lista los nombres que llevan a un rango y salta al elegido', () => {
  const modelo = {
    active: 0,
    sheets: [
      { name: 'Hoja1', cells: {} },
      { name: 'Datos', cells: {} },
    ],
    names: [
      { name: 'Zona', ref: 'Datos!$C$3:$E$4' },
      { name: 'IVA', ref: '0.21' },
    ],
  } as unknown as SheetModel;
  const h = montar(modelo);
  const boton = h.querySelector<HTMLButtonElement>(
    `button[aria-label="${t('sheet.nameBox.names')}"]`,
  )!;
  act(() => boton.click());
  const items = [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')];
  expect(items.map((i) => i.textContent)).toEqual(['Zona']); // IVA es una constante
  act(() => items[0]!.click());
  expect(guardado!.active).toBe(1);
  expect(cuadro(h).value).toBe('C3:E4');
});

test('sin nombres definidos, el cuadro no enseña el ▾', () => {
  const h = montar(hoja({}));
  expect(h.querySelector(`button[aria-label="${t('sheet.nameBox.names')}"]`)).toBeNull();
});
