/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tests unitarios para el despachador de atajos de teclado del editor de Pages.
 */

import { describe, expect, it } from 'bun:test';
import { clasificarAtajo, procesarAtajo } from './atajos';

describe('Atajos de teclado del editor', () => {
  it('identifica atajos con modificador (Ctrl/Cmd)', () => {
    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: 'z',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('deshacer');

    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: true,
        key: 'z',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('rehacer');

    expect(
      clasificarAtajo({
        ctrlKey: false,
        metaKey: true,
        altKey: false,
        shiftKey: false,
        key: 'y',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('rehacer');

    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: 'd',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('duplicar');

    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: 'g',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('envolverMarco');

    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: true,
        key: 'g',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('desenvolverMarco');

    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: '0',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('zoomAjustar');

    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: '1',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('zoom100');
  });

  it('identifica teclas de borrado y escape sin modificador', () => {
    expect(
      clasificarAtajo({
        ctrlKey: false,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: 'Delete',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('eliminar');

    expect(
      clasificarAtajo({
        ctrlKey: false,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: 'Backspace',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('eliminar');

    expect(
      clasificarAtajo({
        ctrlKey: false,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: 'Escape',
        target: null,
      } as unknown as KeyboardEvent),
    ).toBe('escape');
  });

  it('ignora atajos cuando el foco está en un campo de texto', () => {
    const inputMock = {
      tagName: 'INPUT',
      isContentEditable: false,
    } as unknown as EventTarget;

    expect(
      clasificarAtajo({
        ctrlKey: true,
        metaKey: false,
        altKey: false,
        shiftKey: false,
        key: 'z',
        target: inputMock,
      } as unknown as KeyboardEvent),
    ).toBeNull();
  });

  it('ejecuta callbacks en procesarAtajo y cancela el evento por defecto', () => {
    let deshacerLlamado = false;
    let prevenido = false;

    const ev = {
      ctrlKey: true,
      metaKey: false,
      altKey: false,
      shiftKey: false,
      key: 'z',
      target: null,
      preventDefault: () => {
        prevenido = true;
      },
    } as unknown as KeyboardEvent;

    const consumido = procesarAtajo(ev, {
      onDeshacer: () => {
        deshacerLlamado = true;
      },
    });

    expect(consumido).toBe(true);
    expect(deshacerLlamado).toBe(true);
    expect(prevenido).toBe(true);
  });
});
