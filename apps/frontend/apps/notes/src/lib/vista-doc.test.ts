/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import { conLetraDeDocs } from './vista-doc.js';

const FUENTE = 'data:font/woff2;base64,AAAA';

describe('la vista previa de una nota doc lleva la letra de Docs', () => {
  test('la hoja entra en el <head> del export, con la Figtree dentro', () => {
    const html =
      '<!doctype html>\n<html lang="es"><head><meta charset="utf-8">\n<title>t</title></head><body><p>a</p></body></html>';
    const out = conLetraDeDocs(html, FUENTE);
    expect(out.indexOf('<style>')).toBeLessThan(out.indexOf('</head>'));
    expect(out).toContain(`src:url(${FUENTE})`);
    expect(out).toContain("font-family:'Figtree Variable'");
    expect(out).toContain('<body><p>a</p></body>');
  });

  test('sin <head>, la hoja va delante', () => {
    expect(conLetraDeDocs('<p>a</p>', FUENTE).startsWith('<style>')).toBe(true);
  });
});
