/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los enlaces que el selector guarda en una nota. Sin el prefijo de la app
 * (`https://drive.sytue.eu/files/…`) cada uno era el 404 de Traefik en
 * producción: medido con `curl` el 2026-09-23 en Drive, Docs, Calendar y Rooms.
 * Con él (`/drive/files/…`) contestan 302 al login, que es la página que existe.
 */
import { expect, test } from 'bun:test';
import { appDomains } from '@sytue/config';
import type { Channel, DocumentMeta, DriveResource, Event, NoteHit } from '@sytue/api-clients';
import {
  deCalendar,
  deDocs,
  deDrive,
  deNotes,
  deRooms,
  listarDrive,
  mensajeNota,
} from './elegibles.js';

test('cada enlace lleva el host Y el prefijo de su app', () => {
  const fichero = { id: 'f1', name: 'a.pdf', path: '/a.pdf', isDirectory: false } as DriveResource;
  expect(deDrive(fichero).url).toBe(`${appDomains.drive}/drive/files/f1`);
  expect(deDocs({ id: 'd1', title: 'Acta', kind: 'document' } as DocumentMeta).url).toBe(
    `${appDomains.docs}/docus/edit?id=d1`,
  );
  expect(deCalendar({ id: 'e1', uid: 'u1', summary: 'Junta' } as Event).url).toBe(
    `${appDomains.calendar}/calendar/events/e1`,
  );
  expect(deRooms({ id: 'c1', name: 'General' } as Channel).url).toBe(
    `${appDomains.rooms}/rooms/channels/c1`,
  );
  expect(deNotes({ id: 'n1', notebook_id: 'k1', notebook: 'Casa', title: '' } as NoteHit).url).toBe(
    `${appDomains.notes}/notes/?notebook=k1&note=n1`,
  );
});

test('una carpeta de Drive no lleva enlace: se navega, no se elige', () => {
  const carpeta = { id: 'k', name: 'Actas', path: '/Actas', isDirectory: true } as DriveResource;
  expect(deDrive(carpeta).url).toBeUndefined();
});

test('con término, Drive BUSCA en toda la unidad en vez de listar la raíz', async () => {
  const pedido: string[] = [];
  const drive = {
    listPage: async (path: string) => {
      pedido.push(`listar ${path}`);
      return { items: [] };
    },
    search: async (q: string) => {
      pedido.push(`buscar ${q}`);
      return [{ id: 'f9', name: 'informe.pdf', path: '/x/informe.pdf', isDirectory: false }];
    },
  };
  const r = await listarDrive(drive, { path: '/', q: 'informe' });
  expect(pedido).toEqual(['buscar informe']);
  expect(r.items.map((i) => i.name)).toEqual(['informe.pdf']);

  // Sin término sigue siendo el explorador por carpetas.
  await listarDrive(drive, { path: '/Actas', q: '' });
  expect(pedido.at(-1)).toBe('listar /Actas');
});

// Compartir en Rooms enlazaba a `notes.sytue.eu/notebooks/…`: sin el prefijo, el
// 404 de Traefik. Es el mismo enlace que escribe la app nativa (`MensajeNota`).
test('compartir una nota publica su título y el enlace que existe', () => {
  expect(mensajeNota('  Acta  ', 'k1', 'n1')).toBe(
    `📝 Acta\n${appDomains.notes}/notes/?notebook=k1&note=n1`,
  );
  expect(mensajeNota('', 'k1', 'n1')).toStartWith('📝 Nota\n');
});

// La org que compone (la de la sesión) va en el path (ADR 0056): la URL sale
// canónica y aterriza sin el 302 del middleware.
test('con org, los enlaces salen canónicos: el segmento tras el prefijo', () => {
  expect(deDocs({ id: 'd1', title: 'Acta', kind: 'document' } as DocumentMeta, 'acme').url).toBe(
    `${appDomains.docs}/docus/acme/edit?id=d1`,
  );
  expect(deCalendar({ id: 'e1', uid: 'u1', summary: 'Junta' } as Event, 'acme').url).toBe(
    `${appDomains.calendar}/calendar/acme/events/e1`,
  );
  expect(deRooms({ id: 'c1', name: 'General' } as Channel, 'acme').url).toBe(
    `${appDomains.rooms}/rooms/acme/channels/c1`,
  );
  expect(
    deNotes({ id: 'n1', notebook_id: 'k1', notebook: 'Casa', title: '' } as NoteHit, 'acme').url,
  ).toBe(`${appDomains.notes}/notes/acme/?notebook=k1&note=n1`);
  expect(mensajeNota('Acta', 'k1', 'n1', 'acme')).toBe(
    `📝 Acta\n${appDomains.notes}/notes/acme/?notebook=k1&note=n1`,
  );
});
