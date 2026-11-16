/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El registro da `grafico` a la presentación, pero la cinta solo lo pintaba en
 * Insertar del documento: en Diapos vivía únicamente en la mini-barra del lienzo
 * y quien lo buscaba donde está en Docus no lo encontraba. Lo que se fija aquí es
 * que Insertar de la presentación tiene su grupo «Gráfico», al lado de SmartArt,
 * y que su botón manda el mismo comando que la mini-barra.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, spyOn, test } from 'bun:test';
import { render } from 'preact';
import * as lienzo from '../../slides/SlidesCanvas';
import type { EditorContext } from '../../types';
import { InsertarTab } from './Insertar';

if (typeof document === 'undefined') GlobalRegistrator.register();

const montar = (kind: EditorContext['kind']): HTMLElement => {
  document.body.innerHTML = '';
  const host = document.body.appendChild(document.createElement('div'));
  const ctx = { kind, exec: () => {}, fmt: {} } as unknown as EditorContext;
  render(<InsertarTab ctx={ctx} />, host);
  return host;
};

const botonGrafico = (host: HTMLElement): HTMLButtonElement | null =>
  host.querySelector<HTMLButtonElement>('button[aria-label="Insert chart"]');

afterEach(() => {
  document.body.innerHTML = '';
});

test('Insertar de la presentación tiene el grupo «Gráfico», junto a SmartArt', () => {
  const host = montar('presentation');
  const boton = botonGrafico(host);
  expect(boton).not.toBeNull();
  const grupos = [...host.querySelectorAll('[role="group"]')].map((g) =>
    g.getAttribute('aria-label'),
  );
  const grafico = grupos.indexOf('Chart');
  expect(grafico).toBeGreaterThan(-1);
  expect(grupos[grafico + 1]).toBe('SmartArt');
});

test('el botón manda el comando `grafico` al lienzo, como la mini-barra', () => {
  const espia = spyOn(lienzo, 'dispatchSlidesCommand').mockImplementation(() => {});
  try {
    const boton = botonGrafico(montar('presentation'));
    boton?.click();
    expect(espia).toHaveBeenCalledTimes(1);
    expect(espia.mock.calls[0]?.[0]).toBe('grafico');
  } finally {
    espia.mockRestore();
  }
});
