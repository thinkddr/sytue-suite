/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ctrl/⌘ +, − y 0 sobre el zoom del documento (atajosZoom.ts) y el «100 %» de la barra
 * de estado, que vuelve al 100 con un clic (layout/StatusBar.tsx). Docus, Calcus y
 * Diapos comparten los dos.
 *
 * VISTO EN ROJO (2026-11-05): con el porcentaje como <span>, no había botón que pulsar;
 * con `zoomDeAtajo` sin la rama de `code`, Ctrl+0 en un AZERTY («à») no hacía nada.
 * VISTO EN ROJO (2026-11-06) el enganche en window (atajos.ts): sin el
 * `preventDefault`, Ctrl+= seguía llegando al navegador y la prueba de la función
 * pura no se enteraba.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { docsT } from '../i18n';
import { useAtajosDelEditor } from './atajos';
import { zoomDeAtajo, type TeclaZoom } from './atajosZoom';
import { StatusBar } from './layout/StatusBar';
import type { EditorContext, EditorUi } from './types';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tecla = (key: string, extra: Partial<TeclaZoom> = {}): TeclaZoom => ({
  key,
  code: '',
  ctrlKey: true,
  metaKey: false,
  altKey: false,
  ...extra,
});

test('Ctrl/⌘ + y − mueven el zoom de 10 en 10, sin salirse del rango', () => {
  expect(zoomDeAtajo(tecla('+'), 100)).toBe(110);
  expect(zoomDeAtajo(tecla('='), 100)).toBe(110); // el + sin Mayús
  expect(zoomDeAtajo(tecla('-', { ctrlKey: false, metaKey: true }), 100)).toBe(90);
  expect(zoomDeAtajo(tecla('_'), 100)).toBe(90); // el − con Mayús
  expect(zoomDeAtajo(tecla('+'), 200)).toBe(200);
  expect(zoomDeAtajo(tecla('-'), 25)).toBe(25);
});

test('Ctrl/⌘ 0 vuelve al 100, también en un teclado donde el 0 no es «0»', () => {
  expect(zoomDeAtajo(tecla('0'), 150)).toBe(100);
  expect(zoomDeAtajo(tecla('à', { code: 'Digit0' }), 150)).toBe(100);
  expect(zoomDeAtajo(tecla('0', { code: 'Numpad0' }), 40)).toBe(100);
});

test('sin Ctrl/⌘, o con Alt, la tecla no es del zoom: se escribe', () => {
  expect(zoomDeAtajo(tecla('+', { ctrlKey: false }), 100)).toBeNull();
  expect(zoomDeAtajo(tecla('0', { altKey: true }), 100)).toBeNull();
  expect(zoomDeAtajo(tecla('s'), 100)).toBeNull();
});

test('el porcentaje de la barra de estado es un botón que vuelve al 100 %', () => {
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  const pedidos: Partial<EditorUi>[] = [];
  const ctx = {
    t: docsT('es'),
    locale: 'es',
    kind: 'spreadsheet',
    stats: { page: 1, pages: 1, words: 0 },
    ui: { zoom: 130 },
    setUi: (p: Partial<EditorUi>) => pedidos.push(p),
  } as unknown as EditorContext;
  render(<StatusBar ctx={ctx} />, host);
  const boton = [...host.querySelectorAll('button')].find((b) => b.textContent?.trim() === '130 %');
  expect(boton).toBeDefined();
  expect(boton!.getAttribute('aria-label')).toBe('130 % · Volver al 100 %');
  boton!.click();
  expect(pedidos).toEqual([{ zoom: 100 }]);
});

test('Ctrl+= en la ventana amplía el documento y no la página', () => {
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  let ui: EditorUi | null = null;
  let guardados = 0;
  const guardar = (): void => {
    guardados++;
  };
  function Banco() {
    const [estado, setEstado] = useState({ zoom: 100, panel: null } as EditorUi);
    useEffect(() => {
      ui = estado;
    }, [estado]);
    useAtajosDelEditor(guardar, setEstado);
    return null;
  }
  // act: Preact engancha los efectos después del pintado, y sin él no habría oyente.
  act(() => render(<Banco />, host));
  const pulsa = (init: KeyboardEventInit): KeyboardEvent => {
    const ev = new KeyboardEvent('keydown', { cancelable: true, ...init });
    act(() => {
      window.dispatchEvent(ev);
    });
    return ev;
  };
  const mas = pulsa({ key: '=', ctrlKey: true });
  expect(mas.defaultPrevented).toBe(true);
  expect(ui!.zoom).toBe(110);
  expect(pulsa({ key: '0', metaKey: true }).defaultPrevented).toBe(true);
  expect(ui!.zoom).toBe(100);
  // Sin Ctrl/⌘ la tecla es del texto: ni se toca el zoom ni se le quita al navegador.
  expect(pulsa({ key: '=' }).defaultPrevented).toBe(false);
  expect(ui!.zoom).toBe(100);
  // Ctrl+S adelanta el autoguardado en vez de descargar la página.
  expect(pulsa({ key: 's', ctrlKey: true }).defaultPrevented).toBe(true);
  expect(guardados).toBe(1);
  render(null, host);
});
