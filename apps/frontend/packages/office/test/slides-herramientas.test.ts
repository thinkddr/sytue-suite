/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo nuevo de presentar con Diapos contra los VECTORES COMPARTIDOS con la app nativa
 * (`vectores/diapos-herramientas.json`, que Go recorre en
 * `apps/fyne/diapos/herramientas_test.go`): el avance automático en bucle, la rueda, el
 * cronómetro del moderador y las notas del orador; y las teclas L y Ctrl+D.
 *   cd apps/frontend && bun test packages/office/test/slides-herramientas.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  alternarPausa,
  avanceAutomatico,
  notasComoTexto,
  pasoDeRueda,
  transcurrido,
  type Cronometro,
} from '../src/editor/slides/presentar';
import {
  esDuplicarDiapo,
  presentCommand,
  presentShortcuts,
} from '../src/editor/slides/SlidesCanvas';
import { docsT } from '../src/i18n/index';
import type { Slide } from '../src/editor/types';

interface Vectores {
  avance: { ocultas: boolean[]; desde: number; sale: number }[];
  rueda: { acumulado: number; abajo: number; umbral: number; dir: -1 | 0 | 1; resto: number }[];
  crono: (Cronometro & { ahora: number; transcurrido: number })[];
  pausa: (Cronometro & { ahora: number; sale: Cronometro })[];
  notas: { notas: string[]; sale: string }[];
}
const v = JSON.parse(
  readFileSync(new URL('./vectores/diapos-herramientas.json', import.meta.url), 'utf8'),
) as Vectores;

const diapos = (ocultas: boolean[], notas: string[] = []): Slide[] =>
  (ocultas.length > 0 ? ocultas : notas.map(() => false)).map((o, i) => ({
    id: `s${i}`,
    boxes: [],
    ...(o ? { hidden: true } : {}),
    ...(notas[i] !== undefined ? { notes: notas[i] } : {}),
  }));

describe('los vectores compartidos con la app nativa', () => {
  for (const c of v.avance) {
    test(`avance automático desde ${c.desde} con ${JSON.stringify(c.ocultas)}`, () => {
      expect(avanceAutomatico(diapos(c.ocultas), c.desde, 0)).toEqual({ slide: c.sale, step: 0 });
    });
  }
  test('la rueda', () => {
    for (const c of v.rueda) {
      expect(pasoDeRueda(c.acumulado, c.abajo, c.umbral)).toEqual({ dir: c.dir, resto: c.resto });
    }
  });
  test('el cronómetro con su pausa', () => {
    for (const c of v.crono) expect(transcurrido(c, c.ahora)).toBe(c.transcurrido);
    for (const c of v.pausa) expect(alternarPausa(c, c.ahora)).toEqual(c.sale);
  });
  test('las notas del orador', () => {
    for (const c of v.notas) {
      expect(notasComoTexto(diapos([], c.notas), (n) => `Diapositiva ${n}`)).toBe(c.sale);
    }
  });
});

describe('las teclas', () => {
  const estado = { idx: 0, last: 2, digits: '', blanked: false };
  test('L enciende y apaga el puntero láser, y sale en la lista de teclas', () => {
    expect(presentCommand('l', estado).cmd).toEqual({ kind: 'laser' });
    expect(presentCommand('L', estado).cmd).toEqual({ kind: 'laser' });
    // Con la pantalla apagada, L la devuelve y no hace nada más.
    expect(presentCommand('l', { ...estado, blanked: true }).cmd).toEqual({ kind: 'resume' });
    const teclas = presentShortcuts(docsT('es')).flatMap((g) => g.atajos);
    expect(teclas.find((a) => a.teclas.includes('L'))?.descripcion).toBe('Puntero láser');
  });
  test('Ctrl+D (o ⌘D) sobre la tira duplica; con Mayús o Alt, no', () => {
    const tecla = { key: 'd', ctrlKey: true, metaKey: false, shiftKey: false, altKey: false };
    expect(esDuplicarDiapo(tecla)).toBe(true);
    expect(esDuplicarDiapo({ ...tecla, ctrlKey: false, metaKey: true, key: 'D' })).toBe(true);
    expect(esDuplicarDiapo({ ...tecla, shiftKey: true })).toBe(false);
    expect(esDuplicarDiapo({ ...tecla, altKey: true })).toBe(false);
    expect(esDuplicarDiapo({ ...tecla, ctrlKey: false })).toBe(false);
  });
});
