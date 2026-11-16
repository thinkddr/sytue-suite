/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de la fase Slides-Presentación: mapa de
 * teclado del modo presentación (presentKeyTarget) y reordenación de
 * diapositivas desde el carril (reorderSlides).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  presentCommand,
  presentKeyTarget,
  presentShortcuts,
  reorderSlides,
  slideFromDigits,
} from '../src/editor/slides/SlidesCanvas';
import { docsT } from '../src/i18n/index';
import type { Slide } from '../src/editor/types';

const deck = (n: number): Slide[] =>
  Array.from({ length: n }, (_, i) => ({ id: `s${i}`, boxes: [] }));

test('presentKeyTarget: avanza (→/Espacio/AvPág) y retrocede (←/RePág) con topes', () => {
  expect(presentKeyTarget('ArrowRight', 0, 4)).toBe(1);
  expect(presentKeyTarget(' ', 2, 4)).toBe(3);
  expect(presentKeyTarget('PageDown', 3, 4)).toBe(4);
  expect(presentKeyTarget('ArrowRight', 4, 4)).toBe(4); // tope final: no pasa de la última
  expect(presentKeyTarget('ArrowLeft', 3, 4)).toBe(2);
  expect(presentKeyTarget('PageUp', 1, 4)).toBe(0);
  expect(presentKeyTarget('ArrowLeft', 0, 4)).toBe(0); // tope inicial
});

test('presentKeyTarget: Home/End saltan, Escape sale y el resto no navega', () => {
  expect(presentKeyTarget('Home', 3, 4)).toBe(0);
  expect(presentKeyTarget('End', 0, 4)).toBe(4);
  expect(presentKeyTarget('Escape', 2, 4)).toBe('exit');
  expect(presentKeyTarget('a', 2, 4)).toBeNull();
  expect(presentKeyTarget('F1', 2, 4)).toBeNull();
});

/* La tabla de PowerPoint/Keynote. Es la MISMA que `TestLasTeclasDePresentacionSonLasDeLaWeb`
 * en apps/fyne/diapos: si cambia aquí, cambia allí. */
test('presentKeyTarget: ↓ e Intro avanzan, ↑ y Retroceso retroceden, con los mismos topes', () => {
  expect(presentKeyTarget('ArrowDown', 0, 4)).toBe(1);
  expect(presentKeyTarget('Enter', 2, 4)).toBe(3);
  expect(presentKeyTarget('ArrowDown', 4, 4)).toBe(4);
  expect(presentKeyTarget('ArrowUp', 2, 4)).toBe(1);
  expect(presentKeyTarget('Backspace', 1, 4)).toBe(0);
  expect(presentKeyTarget('Backspace', 0, 4)).toBe(0);
});

test('slideFromDigits: número 1-based → índice, con tope y sin diapositiva cero', () => {
  expect(slideFromDigits('3', 9)).toBe(2);
  expect(slideFromDigits('12', 9)).toBe(9); // más allá de la última: la última
  expect(slideFromDigits('007', 9)).toBe(6);
  expect(slideFromDigits('0', 9)).toBeNull();
  expect(slideFromDigits('', 9)).toBeNull();
});

const st = { idx: 2, last: 9, digits: '', blanked: false };

test('presentCommand: las flechas, Intro y Retroceso avanzan y retroceden paso a paso', () => {
  for (const k of ['ArrowRight', 'ArrowDown', ' ', 'Enter']) {
    expect(presentCommand(k, st).cmd).toEqual({ kind: 'advance', dir: 1 });
  }
  for (const k of ['ArrowLeft', 'ArrowUp', 'Backspace']) {
    expect(presentCommand(k, st).cmd).toEqual({ kind: 'advance', dir: -1 });
  }
  // AvPág, Inicio y Fin siguen saltando diapositivas enteras; Escape sale.
  expect(presentCommand('PageDown', st).cmd).toEqual({ kind: 'go', to: 3 });
  expect(presentCommand('End', st).cmd).toEqual({ kind: 'go', to: 9 });
  expect(presentCommand('Escape', st).cmd).toEqual({ kind: 'exit' });
});

test('presentCommand: número + Intro va a esa diapositiva', () => {
  let s = st;
  for (const k of ['1', '2']) {
    const r = presentCommand(k, s);
    expect(r.cmd).toBeNull(); // una cifra sola no mueve nada
    s = { ...s, digits: r.digits };
  }
  expect(s.digits).toBe('12');
  const fin = presentCommand('Enter', { ...s, last: 20 });
  expect(fin).toEqual({ cmd: { kind: 'go', to: 11 }, digits: '' });
  // Otra tecla entre medias olvida el número: «3», «→», «Intro» es avanzar dos veces.
  const r = presentCommand('ArrowRight', { ...st, digits: '3' });
  expect(r.digits).toBe('');
  // «0 + Intro» no lleva a ninguna parte ni avanza.
  expect(presentCommand('Enter', { ...st, digits: '0' })).toEqual({ cmd: null, digits: '' });
  // Mayúsculas no borra el número a medio teclear.
  expect(presentCommand('Shift', { ...st, digits: '4' }).digits).toBe('4');
});

test('presentCommand: B y «.» apagan en negro, W y «,» en blanco; cualquier tecla vuelve', () => {
  for (const k of ['b', 'B', '.']) {
    expect(presentCommand(k, st).cmd).toEqual({ kind: 'blank', color: 'black' });
  }
  for (const k of ['w', 'W', ',']) {
    expect(presentCommand(k, st).cmd).toEqual({ kind: 'blank', color: 'white' });
  }
  const apagada = { ...st, blanked: true };
  // Vuelve y NO hace nada más: ni avanzar, ni salir, ni apagarse otra vez.
  for (const k of ['ArrowRight', 'Escape', 'b', 'x', '5', 'Enter']) {
    expect(presentCommand(k, apagada).cmd).toEqual({ kind: 'resume' });
  }
  expect(presentCommand('Shift', apagada).cmd).toBeNull();
});

test('presentCommand: «?» y H abren la lista; N y P conservan su sentido', () => {
  expect(presentCommand('?', st).cmd).toEqual({ kind: 'help' });
  expect(presentCommand('h', st).cmd).toEqual({ kind: 'help' });
  expect(presentCommand('H', st).cmd).toEqual({ kind: 'help' });
  expect(presentCommand('n', st).cmd).toEqual({ kind: 'notes' });
  expect(presentCommand('P', st).cmd).toEqual({ kind: 'presenter' });
  expect(presentCommand('a', st).cmd).toBeNull();
});

test('presentShortcuts: la lista nombra las teclas en el idioma de quien presenta', () => {
  const grupos = presentShortcuts(docsT('es'));
  const filas = grupos.flatMap((g) => g.atajos);
  expect(filas.some((a) => a.teclas.includes('Intro') && a.teclas.includes('Número'))).toBe(true);
  expect(filas.some((a) => a.teclas.includes('B') && a.descripcion === 'Pantalla negra')).toBe(
    true,
  );
  // Ninguna clave sin traducir se cuela como texto.
  for (const a of filas) {
    for (const k of a.teclas) expect(k.startsWith('slides.')).toBe(false);
    expect(a.descripcion.startsWith('slides.')).toBe(false);
  }
});

test('reorderSlides: mueve conservando la identidad de los objetos y sin mutar', () => {
  const d = deck(4);
  const out = reorderSlides(d, 0, 2);
  expect(out.map((s) => s.id)).toEqual(['s1', 's2', 's0', 's3']);
  expect(out[2]).toBe(d[0]!); // misma referencia: la activa se sigue con findIndex
  expect(d.map((s) => s.id)).toEqual(['s0', 's1', 's2', 's3']); // pura: el original intacto
  // Mover hacia atrás también funciona.
  expect(reorderSlides(d, 3, 0).map((s) => s.id)).toEqual(['s3', 's0', 's1', 's2']);
});

test('reorderSlides: `from` fuera de rango devuelve el array original', () => {
  const d = deck(2);
  expect(reorderSlides(d, 9, 0)).toBe(d);
});
