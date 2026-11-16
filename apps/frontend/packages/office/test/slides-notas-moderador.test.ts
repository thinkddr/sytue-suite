/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Notas del ponente en la vista del moderador (DIAP-08): el tamaño de letra con A−/A+ en
 * la escala de tokens (`text-sm` … `text-2xl`), recordado en `localStorage` y a prueba de
 * almacenamiento bloqueado; y unas notas largas que se desplazan en su franja sin llevarse
 * los botones.
 *   cd apps/frontend && bun test packages/office/test/slides-notas-moderador.test.ts
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, test } from 'bun:test';
import { h, render } from 'preact';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { NOTES_SIZES, PresenterView, readNotesSize, saveNotesSize, stepNotesSize } =
  await import('../src/editor/slides/SlidesCanvas');
const { docsT } = await import('../src/i18n/index');
const { EditorI18nCtx } = await import('../src/editor/context');
import type { NotesSize } from '../src/editor/slides/SlidesCanvas';
import type { SlidesModel } from '../src/editor/types';

afterEach(() => {
  localStorage.clear();
  render(null, document.body);
});

describe('tamaño de las notas', () => {
  test('la escala va de text-sm a text-2xl y cada paso se para en los topes', () => {
    expect([...NOTES_SIZES]).toEqual(['sm', 'base', 'md', 'lg', 'xl', '2xl']);
    expect(stepNotesSize('md', 1)).toBe('lg');
    expect(stepNotesSize('md', -1)).toBe('base');
    expect(stepNotesSize('2xl', 1)).toBe('2xl');
    expect(stepNotesSize('sm', -1)).toBe('sm');
  });

  test('se recuerda entre aperturas; el de siempre no deja rastro', () => {
    expect(readNotesSize()).toBe('md');
    saveNotesSize('xl');
    expect(readNotesSize()).toBe('xl');
    saveNotesSize('md');
    expect(localStorage.length).toBe(0);
    expect(readNotesSize()).toBe('md');
  });

  test('un valor ajeno o un almacenamiento que falla dan el tamaño de siempre', () => {
    localStorage.setItem('sytue.slides.notesSize', '9xl');
    expect(readNotesSize()).toBe('md');
    const original = Storage.prototype.getItem;
    const set = Storage.prototype.setItem;
    Storage.prototype.getItem = () => {
      throw new Error('bloqueado');
    };
    Storage.prototype.setItem = () => {
      throw new Error('bloqueado');
    };
    try {
      expect(readNotesSize()).toBe('md');
      expect(() => saveNotesSize('lg')).not.toThrow();
    } finally {
      Storage.prototype.getItem = original;
      Storage.prototype.setItem = set;
    }
  });
});

describe('PresenterView', () => {
  const sesenta = Array.from({ length: 60 }, (_, i) => `Línea ${i + 1} de las notas.`).join('\n');
  const model = { slides: [{ id: 'a', boxes: [], notes: sesenta }] } as unknown as SlidesModel;

  const pintar = (notesSize: NotesSize, onNotesSize: (d: number) => void = () => {}) =>
    render(
      h(
        EditorI18nCtx.Provider,
        { value: docsT('es') },
        h(PresenterView, {
          model,
          cur: 0,
          step: 0,
          elapsedMs: 0,
          clock: '10:00',
          winW: 1040,
          notesSize,
          onNotesSize,
          onPrev: () => {},
          onNext: () => {},
          onReset: () => {},
          onEnd: () => {},
        }),
      ),
      document.body,
    );

  const boton = (rotulo: string) =>
    document.querySelector<HTMLButtonElement>(`button[aria-label="${rotulo}"]`)!;

  test('las sesenta líneas están en una franja que se desplaza, sin los botones dentro', () => {
    pintar('md');
    const franja = document.querySelector<HTMLElement>('[data-presenter-notes]')!;
    expect(franja.style.overflow).toBe('auto');
    expect(franja.style.minHeight).toBe('0px'); // si no, el flex no la deja encoger y nada se desplaza
    expect(franja.textContent).toContain('Línea 60 de las notas.');
    expect(franja.querySelector('button')).toBeNull();
    expect(boton('Ampliar el texto de las notas')).not.toBeNull();
  });

  test('A− y A+ cambian el tamaño, en la escala de tokens, y se apagan en los topes', () => {
    const pasos: number[] = [];
    pintar('lg', (d) => pasos.push(d));
    const p = document.querySelector<HTMLElement>('[data-notes-size]')!;
    expect(p.style.fontSize).toBe('var(--text-lg)');
    boton('Ampliar el texto de las notas').click();
    boton('Reducir el texto de las notas').click();
    expect(pasos).toEqual([1, -1]);

    pintar('2xl');
    expect(boton('Ampliar el texto de las notas').disabled).toBe(true);
    expect(boton('Reducir el texto de las notas').disabled).toBe(false);
    pintar('sm');
    expect(boton('Reducir el texto de las notas').disabled).toBe(true);
    expect(document.querySelector<HTMLElement>('[data-notes-size]')!.style.fontSize).toBe(
      'var(--text-sm)',
    );
  });
});
