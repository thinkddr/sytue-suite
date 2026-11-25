/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Autosuma, «Limpiar texto» y el CSV de Calcus contra los VECTORES COMPARTIDOS con la app
 * nativa (`vectores/calcus-herramientas.json`, que Go recorre en
 * `apps/fyne/calcus/herramientas_test.go`), y los atajos de teclado que los acompañan.
 *   cd apps/frontend && bun test packages/office/test/sheets-herramientas.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  formulaDeAutosuma,
  planDeAutosuma,
  type FuncionAutosuma,
  type TotalAutosuma,
} from '../src/editor/sheets/autosuma';
import { BOM_UTF8, csvDeFilas, nombreCsv } from '../src/editor/sheets/csv';
import { parseRangeRef } from '../src/editor/sheets/format';
import { transformarTextoCelda, type ModoTextoCeldas } from '../src/editor/sheets/textoCeldas';
import { numToCol } from '@sytue/formula/ligero';
import { atajoDeHoja, hojaVecina, type PulsacionHoja } from '../src/editor/sheets/atajosHoja';
import { literalReescribible } from '../src/editor/sheets/textoCeldas';

interface Vectores {
  autosuma: {
    nombre: string;
    numeros: string[];
    llenas: string[];
    sel: string;
    topes?: { cols: number; filas: number };
    sale: { celda: string; rango: string }[] | null;
  }[];
  formulas: { fn: FuncionAutosuma; rango: string; sale: string }[];
  texto: { modo: ModoTextoCeldas; entrada: string; sale: string }[];
  csv: { filas: string[][]; sale: string }[];
  nombres: { base: string; hoja: string; sale: string }[];
}
const vectores = JSON.parse(
  readFileSync(new URL('./vectores/calcus-herramientas.json', import.meta.url), 'utf8'),
) as Vectores;

const rect = (ref: string) => {
  const r = parseRangeRef(ref);
  if (!r) throw new Error(`referencia mala en los vectores: ${ref}`);
  return r;
};
const a1 = (c: number, r: number) => `${numToCol(c)}${r + 1}`;
const legible = (t: TotalAutosuma) => ({
  celda: a1(t.c, t.r),
  rango: `${a1(t.rango.c1, t.rango.r1)}:${a1(t.rango.c2, t.rango.r2)}`,
});

describe('Autosuma con los vectores compartidos', () => {
  for (const caso of vectores.autosuma) {
    test(caso.nombre, () => {
      const numeros = new Set(caso.numeros);
      const llenas = new Set(caso.llenas);
      const plan = planDeAutosuma(
        rect(caso.sel),
        {
          esNumero: (c, r) => numeros.has(a1(c, r)),
          vacia: (c, r) => !numeros.has(a1(c, r)) && !llenas.has(a1(c, r)),
        },
        caso.topes ?? { cols: 16_384, filas: 1_048_576 },
      );
      expect(plan === null ? null : plan.map(legible)).toEqual(caso.sale);
    });
  }
  for (const f of vectores.formulas) {
    test(`fórmula ${f.sale}`, () => {
      expect(formulaDeAutosuma(f.fn, rect(f.rango))).toBe(f.sale);
    });
  }
});

describe('Limpiar texto con los vectores compartidos', () => {
  test('hay casos de los cinco modos', () => {
    expect(new Set(vectores.texto.map((c) => c.modo))).toEqual(
      new Set(['trim', 'sentence', 'lower', 'upper', 'title']),
    );
  });
  for (const caso of vectores.texto) {
    test(`${caso.modo}: ${JSON.stringify(caso.entrada)}`, () => {
      expect(transformarTextoCelda(caso.entrada, caso.modo)).toBe(caso.sale);
    });
  }
});

describe('CSV con los vectores compartidos', () => {
  for (const caso of vectores.csv) {
    test(JSON.stringify(caso.filas), () => {
      expect(csvDeFilas(caso.filas)).toBe(caso.sale);
    });
  }
  for (const caso of vectores.nombres) {
    test(caso.sale, () => {
      expect(nombreCsv(caso.base, caso.hoja)).toBe(caso.sale);
    });
  }
  test('la marca de orden de bytes es la de UTF-8', () => {
    expect(new TextEncoder().encode(BOM_UTF8)).toEqual(new Uint8Array([0xef, 0xbb, 0xbf]));
  });
});

describe('los atajos que acompañan a estas herramientas', () => {
  const pulsar = (key: string, mods: Partial<PulsacionHoja> = {}) =>
    atajoDeHoja({ key, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...mods });
  test('Alt+= es la autosuma, por el carácter o por la tecla (⌥= del Mac escribe «≠»)', () => {
    expect(pulsar('=', { altKey: true })).toBe('autosuma');
    expect(pulsar('=', { altKey: true, shiftKey: true })).toBe('autosuma'); // Alt+Mayús+0, español
    expect(pulsar('≠', { altKey: true, code: 'Equal' })).toBe('autosuma');
    expect(pulsar('=', { altKey: true, ctrlKey: true })).toBeNull(); // AltGr en Windows
  });
  test('Ctrl+AvPág y Ctrl+RePág cambian de hoja, sin dar la vuelta', () => {
    expect(pulsar('PageDown', { ctrlKey: true })).toBe('hojaSiguiente');
    expect(pulsar('PageUp', { ctrlKey: true, shiftKey: true })).toBe('hojaAnterior');
    expect(pulsar('PageDown')).toBeNull(); // sin Ctrl es bajar una pantalla
    expect(hojaVecina(2, 3, 1)).toBe(2);
    expect(hojaVecina(0, 3, -1)).toBe(0);
    expect(hojaVecina(1, 3, 1)).toBe(2);
  });
  test('Control+Mayús+cifra pone el formato de número por la TECLA', () => {
    expect(pulsar('!', { ctrlKey: true, shiftKey: true, code: 'Digit1' })).toBe(
      'formato:thousands',
    );
    expect(pulsar('·', { ctrlKey: true, shiftKey: true, code: 'Digit3' })).toBe('formato:date');
    expect(pulsar('$', { ctrlKey: true, shiftKey: true, code: 'Digit4' })).toBe('formato:currency');
    expect(pulsar('%', { ctrlKey: true, shiftKey: true, code: 'Digit5' })).toBe('formato:percent');
    // Con ⌘ no: ⌘⇧3/4/5 son las capturas del Mac.
    expect(pulsar('%', { metaKey: true, shiftKey: true, code: 'Digit5' })).toBeNull();
  });
});

describe('qué celda se puede reescribir', () => {
  test('solo el texto con las cinco entidades de stripHtml', () => {
    expect(literalReescribible('ana &amp; luis')).toBe(true);
    expect(literalReescribible('caf&eacute;')).toBe(false);
    expect(literalReescribible('uno<br>dos')).toBe(false);
  });
});
