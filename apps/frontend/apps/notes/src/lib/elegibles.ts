/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que `/api/suite` devuelve de cada servicio: un candidato ya normalizado, con
 * el ENLACE público que se guarda en la nota.
 *
 * El enlace sale de `appUrl`, que lleva el prefijo de la app (`/drive/files/…`).
 * Se escribía con el host pelado (`drive.sytue.eu/files/…`), y en producción eso
 * es el 404 de Traefik: solo la raíz exacta se redirige al prefijo. Es la misma
 * cuenta que hace la app nativa en `apps/nucleo/api/elegir.go`, y los dos tienen
 * que escribir el mismo enlace: la nota se guarda desde uno y se abre desde otro.
 *
 * Aparte del endpoint para poder probarlo sin sesión.
 */
import { appUrl, driveFileUrl, enlaceDocumento } from '@sytue/config';
import type { Channel, DocumentMeta, DriveResource, Event, NoteHit } from '@sytue/api-clients';
import { urlDeNota } from '../client/enlaces.js';
import type { Elegible } from '../client/notesApi.js';

/**
 * La organización que llevan los enlaces (ADR 0056): la de la sesión que los
 * compone, que es la del contenido que referencia. Con ella la URL sale canónica
 * (`notes.sytue.eu/notes/<org>/?notebook=…`); sin ella también funciona —el
 * middleware de la app de destino la añade con un 302—. El enlace PÚBLICO de un
 * cuaderno (`/p/<token>`, NotebookSettings) NO la lleva: es para anónimos y ahí
 * la org es inerte.
 */

/** La dirección de una nota: la de la búsqueda, la de Rooms y la de los enlaces entre notas. */
export const urlDeNotaPublica = (notebookId: string, noteId: string, org?: string): string =>
  urlDeNota(appUrl('notes', '', org), notebookId, noteId);

/**
 * Lo que se publica en una sala al compartir una nota: el título y su ENLACE,
 * nunca el contenido. Enlazaba a `/notebooks/…` sin el prefijo de la app, que en
 * producción es el 404 de Traefik; ahora es el mismo enlace que la búsqueda y
 * que el `MensajeNota` de la app nativa.
 */
export function mensajeNota(
  titulo: string,
  notebookId: string,
  noteId: string,
  org?: string,
): string {
  const enlace = urlDeNotaPublica(notebookId, noteId, org);
  return `📝 ${titulo.trim().slice(0, 200) || 'Nota'}\n${enlace}`;
}

export function deDrive(r: DriveResource): Elegible {
  return {
    id: r.id ?? r.path,
    name: r.name,
    isDirectory: r.isDirectory,
    path: r.path,
    ...(r.isDirectory || !r.id ? {} : { url: driveFileUrl(r.id) }),
    ...(r.isDirectory ? {} : { meta: String(r.size ?? 0) }),
  };
}

export function deDocs(d: DocumentMeta, org?: string): Elegible {
  return {
    id: d.id,
    name: d.title,
    url: enlaceDocumento(d.id, d.kind, org),
    meta: d.kind,
  };
}

// La IDENTIDAD que se guarda es el UID (RFC 5545), no el id interno del servicio:
// es lo que la superficie CalDAV de Calendar tiene a mano para preguntar por el
// otro extremo del enlace. El id solo compone la URL.
export function deCalendar(e: Event, org?: string): Elegible {
  return {
    id: e.uid,
    name: e.summary,
    url: appUrl('calendar', `/events/${encodeURIComponent(e.id)}`, org),
    meta: e.start,
  };
}

export function deRooms(c: Channel, org?: string): Elegible {
  return {
    id: c.id,
    name: c.name,
    url: appUrl('rooms', `/channels/${encodeURIComponent(c.id)}`, org),
    meta: c.topic ?? '',
  };
}

export function deNotes(h: NoteHit, org?: string): Elegible {
  return {
    id: h.id,
    name: h.title || h.notebook,
    url: urlDeNotaPublica(h.notebook_id, h.id, org),
    // De qué cuaderno es: dos notas pueden llamarse «Pendientes», y en una lista de
    // resultados el cuaderno es lo único que las distingue.
    meta: h.notebook,
  };
}

/** Lo que hace falta de DriveClient: navegar una carpeta o buscar en toda la unidad. */
interface Unidad {
  listPage: (
    path: string,
    opts: { cursor?: string | undefined },
  ) => Promise<{ items: DriveResource[]; nextCursor?: string }>;
  search: (q: string) => Promise<DriveResource[]>;
}

/**
 * Drive se NAVEGA por carpetas (el selector de incrustar) o se BUSCA por nombre en
 * toda la unidad (la herramienta de enlaces, que manda `q`). Sin esto último, lo
 * escrito se ignoraba y salían siempre los primeros de «Mi unidad».
 */
export async function listarDrive(
  drive: Unidad,
  { path, cursor, q }: { path: string; cursor?: string | undefined; q: string },
): Promise<{ items: Elegible[]; nextCursor: string | null }> {
  if (q) return { items: (await drive.search(q)).map(deDrive), nextCursor: null };
  const page = await drive.listPage(path, { cursor });
  return { items: page.items.map(deDrive), nextCursor: page.nextCursor ?? null };
}
