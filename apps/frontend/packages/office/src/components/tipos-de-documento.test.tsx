/*
 * CADA TIPO, SU DIBUJO Y SU COLOR.
 *
 * Calcus y Diapos son ESTA misma web abierta con `?kind=`, así que lo único que
 * los distingue de Docus en la portada es el color y la silueta. Salían con los
 * de Docus: el tipo se leía de `window.location`, que en SSR no existe, el HTML
 * se servía como «documento» y al hidratar Preact no repara los atributos que
 * difieren —el rótulo se actualizaba y el `style` y la `d` del path no—.
 *
 * Medido en producción el 2026-09-11 bajo `?kind=spreadsheet`:
 * `background:var(--sytue-product-docs, #2968ec)` y la `d` de la hoja.
 *
 * El arreglo es que el tipo llegue del servidor como prop. Lo que se fija aquí es
 * lo que se veía mal: que los tres tipos NO comparten ni dibujo ni color.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { KIND_COLOR, KindGlyph, PdfGlyph } from './DocList.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function pathDe(nodo: preact.VNode): string {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(nodo, host);
  return [...host.querySelectorAll('path')].map((p) => p.getAttribute('d') ?? '').join('|');
}

test('los tres tipos tienen siluetas distintas', () => {
  const doc = pathDe(<KindGlyph kind="document" />);
  const hoja = pathDe(<KindGlyph kind="spreadsheet" />);
  const diapo = pathDe(<KindGlyph kind="presentation" />);
  expect(new Set([doc, hoja, diapo]).size).toBe(3);
  // Y no por un detalle interior: la hoja de cálculo es una rejilla, no un papel.
  expect(hoja).toContain('M4 5h16v14H4');
  expect(diapo).toContain('M4 4h16v11H4');
});

test('los tres tipos tienen colores distintos', () => {
  const { document: d, spreadsheet: s, presentation: p } = KIND_COLOR;
  expect(new Set([d, s, p]).size).toBe(3);
  expect(s).toContain('product-sheets');
  expect(p).toContain('product-slides');
});

test('CertiPDF lleva el estampador, no un documento a secas', () => {
  const pdf = pathDe(<PdfGlyph />);
  const doc = pathDe(<KindGlyph kind="document" />);
  expect(pdf).not.toBe(doc);
  // El trazo del sello es el de `stamp` de @sytue/ui, no un dibujo paralelo.
  expect(pdf).toContain('M4.5 20.5h15');
});
