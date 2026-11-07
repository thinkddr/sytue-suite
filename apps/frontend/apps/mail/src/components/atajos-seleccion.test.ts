/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Atajos de la selección y de los menús del lector: «x» marca la conversación del
 * cursor, Escape vacía la selección, «l» abre «Etiquetar» y «v» «Mover a…». Y los
 * cuatro en la hoja de «?»: un atajo que no está en la hoja no lo descubre nadie.
 *
 * «x» y Escape se deciden en `atajoDeSeleccion` (utils.ts), que se prueba ejecutándola.
 * Que «l» y «v» encuentren los disparadores del lector se prueba montándolo, en
 * `lector-pestana.test.tsx`. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import type { EmailSummary } from '@sytue/api-clients';
import { esAtajoDeApp } from '@sytue/ui';
import { useT } from '../i18n/index.js';
import { gruposDeAtajos } from './hojaDeAtajos.js';
import { atajoDeSeleccion, groupByThread } from './utils.js';

function correo(id: string): EmailSummary {
  return {
    id,
    threadId: `h-${id}`,
    subject: `Asunto ${id}`,
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [],
    cc: [],
    preview: '',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: {},
    hasAttachment: false,
  };
}

const app = await Bun.file(new URL('./MailApp.tsx', import.meta.url)).text();

const lista = groupByThread([correo('a'), correo('b'), correo('c')]);
const ctx = {
  conversaciones: lista,
  abierta: null as string | null,
  cursor: 0,
  soloLectura: false,
  marcadas: 0,
};

test('«x» marca la abierta, o la fila del cursor sin nada abierto, con su índice', () => {
  expect(atajoDeSeleccion('x', { ...ctx, abierta: 'h-b' })).toEqual({
    tipo: 'marcar',
    threadId: 'h-b',
    indice: 1,
  });
  // El cursor de j/k puede quedar más allá del final tras archivar: la última.
  expect(atajoDeSeleccion('x', { ...ctx, cursor: 7 })).toEqual({
    tipo: 'marcar',
    threadId: 'h-c',
    indice: 2,
  });
});

test('«x» no marca nada si la abierta no está en la lista, ni en solo lectura', () => {
  expect(atajoDeSeleccion('x', { ...ctx, abierta: 'h-de-un-aviso' })).toBeNull();
  expect(atajoDeSeleccion('x', { ...ctx, soloLectura: true })).toBeNull();
  expect(atajoDeSeleccion('x', { ...ctx, conversaciones: [] })).toBeNull();
});

test('Escape vacía la selección, y solo si hay algo marcado', () => {
  expect(atajoDeSeleccion('Escape', { ...ctx, marcadas: 2 })).toEqual({ tipo: 'vaciar' });
  expect(atajoDeSeleccion('Escape', ctx)).toBeNull();
});

test('el manejador de MailApp pasa «x» y Escape por `atajoDeSeleccion`', () => {
  expect(app).toContain('atajoDeSeleccion(e.key');
});

// Marcar con el ratón deja el foco en la casilla de la fila, un `<input
// type="checkbox">`: si `esAtajoDeApp` lo tomase por un campo, Escape no llegaba.
test('con la casilla de una fila enfocada, Escape sigue siendo de la app', () => {
  const casilla = { tagName: 'INPUT', type: 'checkbox' };
  const e = { key: 'Escape', ctrlKey: false, metaKey: false, altKey: false, target: casilla };
  expect(esAtajoDeApp(e as unknown as KeyboardEvent)).toBe(true);
});

test('los cuatro están en la hoja de atajos', () => {
  const t = useT('es');
  const hoja = gruposDeAtajos(t).flatMap((g) => g.atajos);
  for (const [tecla, clave] of [
    ['x', 'shortcuts.select'],
    ['Esc', 'shortcuts.clearSelection'],
    ['l', 'shortcuts.label'],
    ['v', 'shortcuts.move'],
  ] as const) {
    expect(hoja).toContainEqual({ teclas: [tecla], descripcion: t(clave) });
  }
});
