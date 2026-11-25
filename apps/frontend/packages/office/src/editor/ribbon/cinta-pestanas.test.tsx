/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA TIRA DE PESTAÑAS DE LA CINTA ES UN `tablist` DE `@sytue/ui`.
 *
 * La cinta llevaba su propia copia del roving tabindex (flechas, Inicio/Fin y el enlace
 * pestaña-panel), la misma que ya da `useTablist` y que usan las tiras de CertiPDF: dos
 * copias del mismo patrón acaban divergiendo. Aquí se fija lo que se ve con el teclado
 * —que no cambia al migrar— y que la copia no vuelve.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { docsT } from '../../i18n';
import type { EditorContext } from '../types';
import { Ribbon } from './Ribbon';
import { EDITORES } from '../tipos/todos';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

/** Un contexto de documento que contesta «nada» a lo que los cuerpos de pestaña pidan. */
function contexto(): EditorContext {
  const datos: Record<string, unknown> = {
    kind: 'document',
    t: docsT('es'),
    connection: 'online',
    title: 'Informe',
  };
  const nada = (): undefined => undefined;
  return new Proxy(datos, {
    get: (o, k) => (typeof k !== 'string' ? undefined : k in o ? o[k] : nada),
  }) as unknown as EditorContext;
}

// Algunos grupos de la cinta (Macros) piden la sesión al montarse. Con happy-dom en
// `about:blank` ese `fetch` relativo lanza fuera del test y, según cuándo caiga, tumba el
// render y se lleva el foco: la prueba fallaba una de cada dos veces. Sin sesión, aquí.
const fetchOriginal = globalThis.fetch;
beforeEach(() => {
  globalThis.fetch = (() =>
    Promise.resolve(new Response('{}', { status: 401 }))) as unknown as typeof fetch;
});

// Se desmonta al acabar cada prueba: si no, el F6 de la cinta anterior sigue escuchando.
let montado: HTMLElement | undefined;

async function montar(): Promise<HTMLElement> {
  document.body.innerHTML = '';
  const host = document.createElement('div');
  montado = host;
  document.body.appendChild(host);
  render(<Ribbon ctx={contexto()} cuerpos={EDITORES.document.cuerpos} />, host);
  await tick();
  return host;
}

afterEach(() => {
  if (montado) render(null, montado);
  montado = undefined;
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
});

const pestanas = (host: HTMLElement): HTMLElement[] => [
  ...host.querySelectorAll<HTMLElement>('[role="tab"]'),
];
const activa = (host: HTMLElement): string | undefined =>
  pestanas(host).find((p) => p.getAttribute('aria-selected') === 'true')?.id;

async function pulsar(host: HTMLElement, key: string): Promise<void> {
  const lista = host.querySelector('[role="tablist"]')!;
  lista.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  await tick();
}

test('solo la pestaña activa es parada de Tab, y el panel la nombra', async () => {
  const host = await montar();
  expect(pestanas(host).map((p) => p.tabIndex)).toEqual([0, -1, -1, -1, -1, -1]);
  const panel = host.querySelector('[role="tabpanel"]')!;
  // Los ids de siempre: las pruebas de extremo a extremo los buscan por nombre.
  expect(panel.id).toBe('ribbon-tabpanel');
  expect(panel.getAttribute('aria-labelledby')).toBe('ribbon-tab-inicio');
  for (const p of pestanas(host)) expect(p.getAttribute('aria-controls')).toBe('ribbon-tabpanel');
});

test('las flechas, Inicio y Fin mueven la pestaña activa y el foco con ella', async () => {
  const host = await montar();
  await pulsar(host, 'ArrowRight');
  expect(activa(host)).toBe('ribbon-tab-insertar');
  expect(document.activeElement?.id).toBe('ribbon-tab-insertar');
  await pulsar(host, 'End');
  expect(activa(host)).toBe('ribbon-tab-vista');
  await pulsar(host, 'ArrowRight');
  expect(activa(host)).toBe('ribbon-tab-inicio');
  await pulsar(host, 'ArrowLeft');
  expect(activa(host)).toBe('ribbon-tab-vista');
  await pulsar(host, 'Home');
  expect(activa(host)).toBe('ribbon-tab-inicio');
  expect(document.activeElement?.id).toBe('ribbon-tab-inicio');
  expect(host.querySelector('[role="tabpanel"]')?.getAttribute('aria-labelledby')).toBe(
    'ribbon-tab-inicio',
  );
});

test('el clic activa la pestaña y F6 vuelve a ella desde fuera de la cinta', async () => {
  const host = await montar();
  pestanas(host)
    .find((p) => p.id === 'ribbon-tab-revisar')!
    .click();
  await tick();
  expect(activa(host)).toBe('ribbon-tab-revisar');
  const fuera = document.createElement('button');
  document.body.appendChild(fuera);
  fuera.focus();
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'F6', bubbles: true }));
  expect(document.activeElement?.id).toBe('ribbon-tab-revisar');
});

test('la cinta no lleva su propia copia del roving tabindex: usa `useTablist`', async () => {
  const fuente = await Bun.file(new URL('./Ribbon.tsx', import.meta.url)).text();
  expect(fuente).toContain('useTablist(');
  // Las flechas horizontales solo las atiende el hook; el menú Archivo usa ↑/↓.
  expect(fuente).not.toContain("'ArrowRight'");
  expect(fuente).not.toContain("'ArrowLeft'");
});
