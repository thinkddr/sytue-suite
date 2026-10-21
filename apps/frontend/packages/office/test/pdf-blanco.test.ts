/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El PDF en blanco que crea «Crear nuevo → PDF».
 *
 * Lo único que no se puede escribir a ojo son los DESPLAZAMIENTOS de la xref, y es
 * justo lo que rompe a un lector estricto sin que se note al mirar el fichero: aquí
 * se sigue cada uno hasta el `N 0 obj` que dice apuntar.
 */
import { describe, expect, it } from 'bun:test';
import { pdfEnBlanco } from '../src/pdf/blanco';

const texto = (b: Uint8Array): string => String.fromCharCode(...b);

describe('pdfEnBlanco', () => {
  it('empieza por la cabecera que exige /api/pdf-abrir antes de guardar nada', () => {
    expect(texto(pdfEnBlanco().slice(0, 5))).toBe('%PDF-');
  });

  it('cada desplazamiento de la xref cae en el objeto que anuncia', () => {
    const s = texto(pdfEnBlanco());
    const offsets = [...s.matchAll(/^(\d{10}) 00000 n $/gm)].map((m) => Number(m[1]));
    expect(offsets).toHaveLength(3);
    offsets.forEach((off, i) => {
      expect(s.slice(off, off + `${i + 1} 0 obj`.length)).toBe(`${i + 1} 0 obj`);
    });
  });

  it('startxref apunta al inicio de la tabla', () => {
    const s = texto(pdfEnBlanco());
    const inicio = Number(/startxref\n(\d+)/.exec(s)![1]);
    expect(s.slice(inicio, inicio + 4)).toBe('xref');
  });

  it('es una sola página A4 y termina en %%EOF', () => {
    const s = texto(pdfEnBlanco());
    expect(s).toContain('/Count 1');
    expect(s).toContain('/MediaBox [0 0 595.28 841.89]');
    expect(s.trimEnd().endsWith('%%EOF')).toBe(true);
  });
});
