/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Vista de conversación ATRAVESANDO carpetas. La lista que la app pliega es de
 * UNA carpeta, así que el hilo llega incompleto: tu respuesta vive en Enviados.
 * `mergeThread` funde el hilo del servidor con lo que la vista ya tiene, y ahí
 * están las dos trampas: no pisar las marcas optimistas locales, y no resucitar
 * dentro de Recibidos lo que mandaste a la papelera.
 * Ejecutar: cd apps/frontend/apps/mail && bun test
 */
import { expect, test } from 'bun:test';
import { groupByThread, mergeThread } from '../src/components/utils.js';
import type { EmailChange, EmailSummary } from '../src/client/mailApi.js';

const INBOX = 'mb-inbox';
const SENT = 'mb-sent';
const TRASH = 'mb-trash';

function msg(id: string, mailboxId: string, extra: Partial<EmailChange> = {}): EmailChange {
  return {
    id,
    threadId: 'hilo-1',
    subject: 'Presupuesto Q3',
    from: [{ email: 'cliente@ext.eu' }],
    to: [],
    cc: [],
    preview: '',
    receivedAt: '2026-08-01T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
    mailboxId,
    ...extra,
  };
}

/** La conversación tal como la arma la vista: solo lo que hay en la carpeta. */
function convDe(...messages: EmailSummary[]) {
  return groupByThread(messages)[0]!;
}

test('el hilo suma lo que vive en otra carpeta (Enviados)', () => {
  const recibido = msg('m1', INBOX);
  const respuesta = msg('m2', SENT, { receivedAt: '2026-08-01T11:00:00Z' });
  const conv = convDe(recibido); // la carpeta Recibidos solo tiene el primero
  expect(conv.messages).toHaveLength(1);

  const shown = mergeThread(conv, [respuesta, recibido], new Set());
  expect(shown.map((m) => m.id)).toEqual(['m2', 'm1']); // orden del servidor: reciente→antiguo
});

test('gana la copia LOCAL: abrir el hilo no revierte lo que acabas de marcar', () => {
  // La vista ya marcó el mensaje como destacado (optimista); el servidor aún no.
  const local = msg('m1', INBOX, { keywords: { $seen: true, $flagged: true } });
  const delServidor = msg('m1', INBOX, { keywords: { $seen: true } });
  const shown = mergeThread(convDe(local), [delServidor], new Set());
  expect(shown).toHaveLength(1);
  expect(shown[0]!.keywords.$flagged).toBe(true);
});

test('lo que está en la Papelera no reaparece dentro del hilo', () => {
  const recibido = msg('m1', INBOX);
  const tirado = msg('m2', TRASH, { receivedAt: '2026-08-01T11:00:00Z' });
  const shown = mergeThread(convDe(recibido), [tirado, recibido], new Set([TRASH]));
  expect(shown.map((m) => m.id)).toEqual(['m1']);
});

test('pero lo que YA está en la vista se queda, aunque su carpeta esté oculta', () => {
  // Caso real: buscar (o abrir una etiqueta) saca resultados de cualquier
  // carpeta, Papelera incluida, mientras la carpeta seleccionada sigue siendo
  // Recibidos —así que la Papelera está en `hidden`—. El mensaje que el usuario
  // acaba de pinchar NO puede desaparecer del lector.
  const tirado = msg('m2', TRASH);
  const shown = mergeThread(convDe(tirado), [tirado], new Set([TRASH]));
  expect(shown.map((m) => m.id)).toEqual(['m2']);
});

test('un hilo que no ha llegado del servidor no borra la conversación', () => {
  const recibido = msg('m1', INBOX);
  // Contrato: mergeThread solo se llama con el hilo ya cargado; con lista vacía
  // devuelve vacío y es la app quien pasa null al lector (se queda la carpeta).
  expect(mergeThread(convDe(recibido), [], new Set())).toEqual([]);
});
