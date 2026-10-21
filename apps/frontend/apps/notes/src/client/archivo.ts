/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El Archivo: qué cuadernos van al árbol y cuáles a la vista «Archivo». Es
 * `archivo.go` en Fyne, con la misma regla.
 */
import type { Notebook } from '@sytue/api-clients';

/** Los que van al árbol: los no archivados. */
export function enArbol(nbs: readonly Notebook[]): Notebook[] {
  return nbs.filter((nb) => !nb.archived_at);
}

/**
 * Los que salen en la vista «Archivo»: solo la CABEZA de cada rama archivada.
 * Un hijo archivado con su padre vuelve cuando vuelve el padre; ofrecerlo suelto
 * lo devolvería al árbol colgando de un padre que no se ve.
 */
export function raicesArchivadas(nbs: readonly Notebook[]): Notebook[] {
  const archivados = new Set(nbs.filter((nb) => nb.archived_at).map((nb) => nb.id));
  return nbs.filter(
    (nb) => archivados.has(nb.id) && !(nb.parent_id && archivados.has(nb.parent_id)),
  );
}
