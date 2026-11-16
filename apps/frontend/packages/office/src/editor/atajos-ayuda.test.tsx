/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja «Atajos de teclado» en marcha: Ctrl+/ en la ventana la abre (y se lo quita al
 * navegador), Escape la cierra y el foco vuelve a donde estaba, y el botón de Vista la
 * abre igual. La lista en sí la prueba test/atajos-ayuda.test.ts.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { ShortcutsModal } from '@sytue/ui';
import { docsT } from '../i18n';
import { useAtajosDelEditor } from './atajos';
import { gruposDeAtajos } from './atajosAyuda';
import { EditorI18nCtx } from './context';
import { VistaTab } from './ribbon/tabs/Vista';
import type { EditorContext, EditorUi } from './types';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');

test('Ctrl+/ abre la hoja; Escape la cierra y devuelve el foco', async () => {
  document.body.innerHTML = '';
  const antes = document.createElement('button');
  antes.textContent = 'antes';
  document.body.appendChild(antes);
  const host = document.createElement('div');
  document.body.appendChild(host);
  let ui: EditorUi | null = null;
  function Banco() {
    const [estado, setEstado] = useState({ zoom: 100, panel: null } as EditorUi);
    useEffect(() => {
      ui = estado;
    }, [estado]);
    useAtajosDelEditor(() => undefined, setEstado);
    return (
      <ShortcutsModal
        open={estado.atajos === true}
        onClose={() => setEstado((prev) => ({ ...prev, atajos: false }))}
        title={t('view.shortcuts')}
        groups={estado.atajos ? gruposDeAtajos('document', t, [], false) : []}
      />
    );
  }
  act(() => render(<Banco />, host));
  antes.focus();
  const ev = new KeyboardEvent('keydown', { key: '/', ctrlKey: true, cancelable: true });
  act(() => {
    window.dispatchEvent(ev);
  });
  // Sin preventDefault, Ctrl+/ seguiría al navegador (o a una extensión).
  expect(ev.defaultPrevented).toBe(true);
  expect(ui!.atajos).toBe(true);
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  const dialogo = document.querySelector('[role="dialog"]');
  expect(dialogo).not.toBeNull();
  expect(dialogo!.textContent).toContain('Atajos de teclado');
  expect(dialogo!.textContent).toContain('Ctrl+Mayús+X');
  // Escape, desde dentro del diálogo, como lo pulsaría quien lo tiene delante.
  const dentro = (document.activeElement ?? dialogo) as HTMLElement;
  act(() => {
    dentro.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(ui!.atajos).toBe(false);
  expect(document.activeElement).toBe(antes);
  act(() => render(null, host));
});

test('Vista › Mostrar tiene el botón de la hoja, con su atajo', () => {
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  // El grupo de macros pregunta por la sesión al montar: aquí no hay servidor.
  const fetchReal = globalThis.fetch;
  globalThis.fetch = (() =>
    Promise.resolve(new Response('{"authenticated":false}'))) as unknown as typeof fetch;
  const pedidos: Partial<EditorUi>[] = [];
  const ctx = {
    t,
    locale: 'es',
    kind: 'document',
    soloLectura: false,
    fmt: { inTable: false, imageSelected: false },
    ui: { zoom: 100, panel: null, ruler: true, settingsOpen: false },
    setUi: (p: Partial<EditorUi>) => pedidos.push(p),
  } as unknown as EditorContext;
  act(() =>
    render(
      <EditorI18nCtx.Provider value={t}>
        <VistaTab ctx={ctx} />
      </EditorI18nCtx.Provider>,
      host,
    ),
  );
  const boton = host.querySelector<HTMLButtonElement>('button[aria-label="Atajos de teclado"]');
  expect(boton).not.toBeNull();
  expect(boton!.getAttribute('aria-keyshortcuts')).toBe('Control+/ Meta+/');
  boton!.click();
  expect(pedidos).toEqual([{ atajos: true }]);
  act(() => render(null, host));
  globalThis.fetch = fetchReal;
});

test('un Ctrl+/ que ya atendió la rejilla de Calcus no vuelve a alternar la hoja', () => {
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  const cambios: boolean[] = [];
  function Banco() {
    const [estado, setEstado] = useState({ zoom: 100, panel: null, atajos: true } as EditorUi);
    useEffect(() => {
      cambios.push(estado.atajos === true);
    }, [estado]);
    useAtajosDelEditor(() => undefined, setEstado);
    return null;
  }
  act(() => render(<Banco />, host));
  // La rejilla la abre por `setUi` y hace preventDefault: alternarla aquí la cerraría.
  const ev = new KeyboardEvent('keydown', { key: '/', ctrlKey: true, cancelable: true });
  ev.preventDefault();
  act(() => {
    window.dispatchEvent(ev);
  });
  expect(cambios).toEqual([true]);
  act(() => render(null, host));
});
