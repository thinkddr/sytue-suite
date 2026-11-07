/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `useEstrecha` ARRANCA COMO EL SERVIDOR Y SIGUE A LA VENTANA.
 *
 * Cambia el MARCADO de una isla `client:load` (el mes de Calendar pasa de rejilla a mes
 * compacto), así que su primer render tiene que ser el del servidor —que no sabe el
 * ancho— o la hidratación vería dos árboles distintos. La respuesta de verdad llega en
 * el efecto, y después sigue a la ventana (girar el teléfono).
 *
 * Visto en ROJO quitando el `false` de `useConsulta(CONSULTA_ESTRECHA, false)`: el primer
 * render ya sale estrecho.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { CONSULTA_ESTRECHA, useEnHoja, useEstrecha } from './useEnHoja.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
const original = globalThis.matchMedia;
afterEach(() => {
  globalThis.matchMedia = original;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

/** Una ventana falsa: todas las consultas responden `ancho < 640`. */
function ventana(estrecha: boolean) {
  const oyentes = new Set<() => void>();
  const consultas: string[] = [];
  const m = {
    matches: estrecha,
    addEventListener: (_: string, f: () => void) => oyentes.add(f),
    removeEventListener: (_: string, f: () => void) => oyentes.delete(f),
  };
  globalThis.matchMedia = ((q: string) => {
    consultas.push(q);
    return m;
  }) as unknown as typeof matchMedia;
  return {
    consultas,
    cambiar(v: boolean) {
      m.matches = v;
      for (const f of oyentes) f();
    },
  };
}

test('useEstrecha: el primer render es el del servidor; el efecto trae la ventana', () => {
  const v = ventana(true);
  const vistos: boolean[] = [];
  function Prueba() {
    vistos.push(useEstrecha());
    return null;
  }
  const host = document.createElement('div');
  act(() => render(<Prueba />, host));
  expect(vistos[0]).toBe(false);
  expect(vistos.at(-1)).toBe(true);
  expect(v.consultas).toContain(CONSULTA_ESTRECHA);
  // Girar el teléfono: sigue a la ventana sin volver a montar.
  act(() => v.cambiar(false));
  expect(vistos.at(-1)).toBe(false);
  render(null, host);
});

test('useEnHoja sigue preguntando ya en el primer render', () => {
  ventana(true);
  const vistos: boolean[] = [];
  function Prueba() {
    vistos.push(useEnHoja());
    return null;
  }
  const host = document.createElement('div');
  act(() => render(<Prueba />, host));
  expect(vistos[0]).toBe(true);
  render(null, host);
});
