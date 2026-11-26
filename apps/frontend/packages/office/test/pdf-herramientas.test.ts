/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El texto fuera del PDF en CertiPDF contra los VECTORES COMPARTIDOS con la app nativa
 * (`vectores/certipdf-herramientas.json`, que Go recorre en
 * `apps/fyne/apps/certipdf/herramientas_test.go`), y los atajos nuevos del visor.
 *   cd apps/frontend && bun test packages/office/test/pdf-herramientas.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  lineasDeTexto,
  resumenDeComentarios,
  textoDelPdf,
  textoParaLeer,
  type AnotacionLeida,
  type Fragmento,
} from '../src/pdf/api';
import { accionDeTecla } from '../src/pdf/atajos';

interface Vectores {
  lineas: { fragmentos: Fragmento[]; sale: string[] }[];
  texto: { paginas: (Fragmento[] | null)[]; sale: string }[];
  resumen: { anotaciones: AnotacionLeida[]; sale: string }[];
}
const v = JSON.parse(
  readFileSync(new URL('./vectores/certipdf-herramientas.json', import.meta.url), 'utf8'),
) as Vectores;

describe('los vectores compartidos con la app nativa', () => {
  test('los renglones de una página, y la voz los une con una pausa', () => {
    for (const c of v.lineas) {
      expect(lineasDeTexto(c.fragmentos)).toEqual(c.sale);
      expect(textoParaLeer(c.fragmentos)).toBe(c.sale.join('. '));
    }
  });
  test('el texto del documento entero', () => {
    for (const c of v.texto) expect(textoDelPdf(c.paginas)).toBe(c.sale);
  });
  test('el resumen de comentarios', () => {
    for (const c of v.resumen) {
      expect(
        resumenDeComentarios(c.anotaciones, {
          pagina: (n) => `Página ${n}`,
          tipo: (k) => k,
          anonimo: 'Anónimo',
        }),
      ).toBe(c.sale);
    }
  });
});

describe('los atajos nuevos del visor', () => {
  const tecla = (key: string, ctrlKey = false) => ({ key, ctrlKey, metaKey: false, altKey: false });
  test('Ctrl+G pide la página; también desde un campo', () => {
    expect(accionDeTecla(tecla('g', true), false)).toBe('irA');
    expect(accionDeTecla(tecla('G', true), true)).toBe('irA');
    expect(accionDeTecla(tecla('g'), false)).toBeNull();
  });
  test('Ctrl+Inicio y Ctrl+Fin van a la primera y a la última, como Inicio y Fin', () => {
    expect(accionDeTecla(tecla('Home', true), false)).toBe('primera');
    expect(accionDeTecla(tecla('End', true), false)).toBe('ultima');
    expect(accionDeTecla(tecla('Home'), false)).toBe('primera');
  });
});
