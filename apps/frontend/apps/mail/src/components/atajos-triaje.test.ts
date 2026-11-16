/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos de Gmail que completan el triaje: `Mayús+I` (leída), `Mayús+U` (no
 * leída), «!» (spam) y «z» (deshacer el último triaje). Qué hace cada tecla se decide
 * en `atajoDeTriaje` (utils.ts), que se prueba ejecutándola; y los cuatro tienen que
 * estar en la hoja de «?», que es donde se aprenden. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import type { EmailSummary } from '@sytue/api-clients';
import { esAtajoDeApp } from '@sytue/ui';
import { useT } from '../i18n/index.js';
import { atajoConMayus, gruposDeAtajos, teclaMayus } from './hojaDeAtajos.js';
import { atajoDeTriaje, groupByThread } from './utils.js';

function correo(id: string, leido: boolean): EmailSummary {
  return {
    id,
    threadId: `h-${id}`,
    subject: `Asunto ${id}`,
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [],
    cc: [],
    preview: '',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: leido ? { $seen: true } : {},
    hasAttachment: false,
  };
}

const app = await Bun.file(new URL('./MailApp.tsx', import.meta.url)).text();
const lector = await Bun.file(new URL('./MessageReader.tsx', import.meta.url)).text();

// a sin leer, b leída.
const lista = groupByThread([correo('a', false), correo('b', true)]);
const [sinLeer, leida] = lista as [(typeof lista)[0], (typeof lista)[0]];
const ctx = {
  conversaciones: lista,
  abierta: null as (typeof lista)[0] | null,
  cursor: 0,
  soloLectura: false,
  hayDeshacer: false,
};

test('Mayús+I marca leída la abierta; Mayús+U, no leída', () => {
  expect(atajoDeTriaje('I', { ...ctx, abierta: sinLeer })).toEqual({
    tipo: 'leida',
    conversacion: sinLeer,
  });
  expect(atajoDeTriaje('U', { ...ctx, abierta: leida })).toEqual({
    tipo: 'noLeida',
    conversacion: leida,
  });
});

test('sin nada abierto actúan sobre la fila del cursor de j/k', () => {
  expect(atajoDeTriaje('U', { ...ctx, cursor: 1 })).toEqual({
    tipo: 'noLeida',
    conversacion: leida,
  });
  // El cursor puede quedar más allá del final tras archivar: la última.
  expect(atajoDeTriaje('!', { ...ctx, cursor: 9 })).toEqual({ tipo: 'spam', conversacion: leida });
  expect(atajoDeTriaje('!', { ...ctx, conversaciones: [] })).toBeNull();
});

test('leída la que ya lo está, o no leída la que no, no es atajo', () => {
  expect(atajoDeTriaje('I', { ...ctx, abierta: leida })).toBeNull();
  expect(atajoDeTriaje('U', { ...ctx, abierta: sinLeer })).toBeNull();
});

test('«z» deshace solo si hay un triaje que deshacer', () => {
  expect(atajoDeTriaje('z', { ...ctx, hayDeshacer: true })).toEqual({ tipo: 'deshacer' });
  expect(atajoDeTriaje('z', ctx)).toBeNull();
});

test('en solo lectura ninguno de los cuatro hace nada', () => {
  const ro = { ...ctx, abierta: sinLeer, soloLectura: true, hayDeshacer: true };
  for (const tecla of ['I', 'U', '!', 'z']) expect(atajoDeTriaje(tecla, ro)).toBeNull();
});

test('las minúsculas siguen siendo de sus atajos: «u» vuelve a la lista', () => {
  expect(atajoDeTriaje('u', { ...ctx, abierta: leida })).toBeNull();
  expect(atajoDeTriaje('i', { ...ctx, abierta: sinLeer })).toBeNull();
});

// En un teclado español «!» es Mayús+1 y `Mayús+I` llega como «I» con shiftKey: si
// `esAtajoDeApp` descartara Shift como descarta Ctrl, ninguno llegaría.
test('con Mayúsculas pulsada, la tecla sigue siendo de la app', () => {
  for (const key of ['I', 'U', '!']) {
    const e = { key, shiftKey: true, ctrlKey: false, metaKey: false, altKey: false, target: null };
    expect(esAtajoDeApp(e as unknown as KeyboardEvent)).toBe(true);
  }
});

test('el manejador de MailApp pasa las cuatro teclas por `atajoDeTriaje` y guarda el último triaje', () => {
  expect(app).toContain('atajoDeTriaje(e.key');
  for (const tecla of ["case 'I':", "case 'U':", "case '!':", "case 'z':"]) {
    expect(app).toContain(tecla);
  }
  expect(app).toContain('ultimoTriaje.current = origenes;');
});

test('los cuatro están en la hoja de atajos, Mayúsculas con el nombre del idioma', () => {
  const t = useT('es');
  const hoja = gruposDeAtajos(t).flatMap((g) => g.atajos);
  const mayus = teclaMayus(t, false);
  expect(mayus).toBe('Mayús');
  for (const [teclas, clave] of [
    [[mayus, 'I'], 'shortcuts.markRead'],
    [[mayus, 'U'], 'shortcuts.markUnread'],
    [['!'], 'shortcuts.spam'],
    [['z'], 'shortcuts.undo'],
  ] as const) {
    expect(hoja).toContainEqual({ teclas: [...teclas], descripcion: t(clave) });
  }
});

test('el tooltip del lector enseña los atajos, con ⇧ en un Mac', () => {
  const t = useT('es');
  expect(atajoConMayus(t, 'U', false)).toBe('Mayús+U');
  expect(atajoConMayus(t, 'U', true)).toBe('⇧U');
  expect(lector).toContain("atajoConMayus(t, 'U')");
  expect(lector).toContain("${t('action.spam')} (!)");
});
