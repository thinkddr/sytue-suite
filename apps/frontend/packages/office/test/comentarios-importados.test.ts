/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de `conservarMarks` y `guardarComentarios`: nunca queda un
 * `<mark data-comment-id>` sin comentario guardado. Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/comentarios-importados.test.ts
 */
import { expect, test } from 'bun:test';
import {
  conservarMarks,
  guardarComentarios,
  type ComentarioImportado,
  type Convertido,
} from '../src/lib/comentarios-importados';

const html =
  '<p>Total <mark data-comment-id="aa11">1200 <strong>euros</strong></mark> y ' +
  '<mark data-comment-id="bb22">otro</mark></p>';

test('conservarMarks deja solo los guardados y conserva el texto de los demás', () => {
  expect(conservarMarks(html, new Set(['aa11']))).toBe(
    '<p>Total <mark data-comment-id="aa11">1200 <strong>euros</strong></mark> y otro</p>',
  );
  expect(conservarMarks(html, new Set())).toBe('<p>Total 1200 <strong>euros</strong> y otro</p>');
});

const conv = (comments?: ComentarioImportado[]): Convertido => ({
  kind: 'document',
  content: html,
  ...(comments ? { comments } : {}),
});
const c = (id: string) => ({ id, text: 'x', anchored: true });

test('guardarComentarios siembra con los marks que el servicio dice guardados', async () => {
  const r = await guardarComentarios(conv([c('aa11'), c('bb22')]), async () => ['bb22']);
  expect(r.fallo).toBe(false);
  expect(r.content).toContain('<mark data-comment-id="bb22">otro</mark>');
  expect(r.content).not.toContain('aa11');
});

test('si no se pueden guardar los comentarios, no queda ningún mark y se avisa', async () => {
  const r = await guardarComentarios(conv([c('aa11')]), async () => {
    throw new Error('red');
  });
  expect(r.fallo).toBe(true);
  expect(r.content).not.toContain('data-comment-id');
});

test('sin comentarios no queda ningún mark', async () => {
  const r = await guardarComentarios(conv(undefined), async () => ['aa11']);
  expect(r.fallo).toBe(false);
  expect(r.content).not.toContain('data-comment-id');
});
