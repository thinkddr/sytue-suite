// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { expect, test } from 'bun:test';
import type { Note } from '@sytue/api-clients';
import { Versiones, conVersion, datosDeCopia, esConflicto, mismoContenido } from './conflicto.js';
import { NotesApiError } from './notesApi.js';

const nota = (extra: Partial<Note> = {}): Note =>
  ({
    id: 'n1',
    notebook_id: 'c1',
    owner_sub: 'ada',
    kind: 'text',
    title: 'Lista',
    content: '<p>leche</p>',
    data: { a: 1, b: [1, 2] },
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 1,
    collapsed: false,
    checked: false,
    pinned: false,
    created_at: '2026-11-09T10:00:00Z',
    updated_at: '2026-11-09T10:00:00.1Z',
    ...extra,
  }) as Note;

test('solo el 409 note_edit_conflict es un conflicto de edición', () => {
  expect(esConflicto(new NotesApiError(409, 'note_edit_conflict'))).toBe(true);
  expect(esConflicto(new NotesApiError(409, 'notebook_crypto_changed'))).toBe(false);
  expect(esConflicto(new NotesApiError(500, 'note_edit_conflict'))).toBe(false);
  expect(esConflicto(new Error('note_edit_conflict'))).toBe(false);
});

test('moverla no es cambiar su contenido; cambiar el texto, los datos o la marca sí', () => {
  const base = nota();
  // El servidor devuelve `data` con las claves en otro orden y la nota en otro sitio.
  const movida = nota({ x: 300, position: 4, updated_at: 'otra', data: { b: [1, 2], a: 1 } });
  expect(mismoContenido(base, movida)).toBe(true);
  expect(mismoContenido(base, nota({ content: '<p>pan</p>' }))).toBe(false);
  expect(mismoContenido(base, nota({ title: 'Otra' }))).toBe(false);
  expect(mismoContenido(base, nota({ data: { a: 2, b: [1, 2] } }))).toBe(false);
  expect(mismoContenido(base, nota({ checked: true }))).toBe(false);
  expect(mismoContenido(base, nota({ content_cipher: 'encv1:x' }))).toBe(false);
});

test('el segundo guardado manda la versión que dejó el primero aunque en pantalla siga la vieja', async () => {
  const v = new Versiones();
  const vista = nota();
  expect(v.base(vista).updated_at).toBe(vista.updated_at);
  const guardada = nota({ content: '<p>leche y pan</p>', updated_at: '2026-11-09T10:00:01Z' });
  v.guardada(vista, guardada);
  // En pantalla sigue la nota de antes (con el texto optimista): manda la guardada.
  expect(v.base({ ...vista, content: '<p>leche y pan</p>' }).updated_at).toBe(guardada.updated_at);
  // Ya se pinta la guardada: también.
  expect(v.base(guardada).updated_at).toBe(guardada.updated_at);
  // Se ha vuelto a leer el cuaderno y trae otra más nueva: cuenta la de pantalla.
  const releida = nota({ updated_at: '2026-11-09T11:00:00Z' });
  expect(v.base(releida).updated_at).toBe(releida.updated_at);
  v.olvidar('n1');
  expect(v.base(vista).updated_at).toBe(vista.updated_at);
});

test('los guardados de una nota salen de uno en uno, aunque el anterior falle', async () => {
  const v = new Versiones();
  const orden: string[] = [];
  let soltar!: () => void;
  const primero = v.enCola('n1', async () => {
    orden.push('primero sale');
    await new Promise<void>((r) => (soltar = r));
    orden.push('primero vuelve');
    throw new Error('red');
  });
  const segundo = v.enCola('n1', async () => {
    orden.push('segundo sale');
    return 2;
  });
  // Otra nota no espera a esta.
  await v.enCola('n2', async () => orden.push('otra nota'));
  expect(orden).toEqual(['primero sale', 'otra nota']);
  soltar();
  await expect(primero).rejects.toThrow('red');
  expect(await segundo).toBe(2);
  expect(orden).toEqual(['primero sale', 'otra nota', 'primero vuelve', 'segundo sale']);
});

test('el guardado lleva la versión sobre la que se editó', () => {
  expect(conVersion({ title: 'a' }, nota()).expected_updated_at).toBe('2026-11-09T10:00:00.1Z');
});

test('la copia de «Guardar las dos» no reutiliza la marca de captura ni los controles de tarea', () => {
  expect(
    datosDeCopia({ client_id: 'x', repeated_into: 'n2', carry_count: 1, attachments: [1] }),
  ).toEqual({ attachments: [1] });
});
