/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los comentarios de `paridad.ts` dicen lo que las nativas HACEN, y las nativas son
 * Fyne. Gio se retiró el 2026-09-23, pero varias entradas seguían contando lo que
 * hacía —«Gio abre la suite actual», «el ERP de Gio es genérico…»— y quien leía la
 * fila de CertiPDF o del tablero del ERP creía que en el escritorio no había nada,
 * cuando el código de Fyne ya estaba hecho. Se mira en el fuente porque los
 * comentarios no llegan al módulo.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const fuente = readFileSync(join(import.meta.dir, 'paridad.ts'), 'utf8');

/** Cada entrada del registro: su `id`, su `producto` y su texto entero, comentarios incluidos. */
function entradas(): { id: string; producto: string; texto: string }[] {
  const out: { id: string; producto: string; texto: string }[] = [];
  const re = /\{\s*\n\s*id: '([^']+)'([\s\S]*?)\n {2}\}/g;
  for (let m = re.exec(fuente); m; m = re.exec(fuente)) {
    const producto = /producto: '([^']+)'/.exec(m[2]!)?.[1] ?? '';
    out.push({ id: m[1]!, producto, texto: m[2]! });
  }
  return out;
}

test('el registro se lee entero (la expresión no se queda corta)', () => {
  const ids = entradas().map((e) => e.id);
  expect(ids).toContain('erp-dashboard-resumen');
  expect(ids).toContain('certipdf-abrir');
  expect(ids).toContain('certipdf-pagina-dibujada');
  expect(ids).toContain('certipdf-firmar');
});

test('las entradas del ERP y de CertiPDF hablan de Fyne, no de Gio', () => {
  const conGio = entradas()
    .filter((e) => e.producto === 'erp' || e.producto === 'pdf')
    .filter((e) => /\bGio\b/.test(e.texto))
    .map((e) => e.id);
  expect(conGio).toEqual([]);
});
