/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La papelera: qué dice cada fila de cuándo se vaciará. Es `caducidadPapelera` de
 * `paneles.go` en Fyne, con la misma regla.
 */
import type { TrashedNote } from '@sytue/api-clients';

const DIA_MS = 24 * 60 * 60 * 1000;

/** Lo que pinta la fila: la clave del catálogo y, si la lleva, la cuenta de días. */
export type Caducidad =
  { clave: 'trash.purgeIn'; dias: number } | { clave: 'trash.purgeSoon' } | { clave: 'trash.kept' };

/**
 * Cuándo se borrará sola. El servicio manda `purge_at` ya calculado con el plazo y las
 * retenciones legales; sin él, la nota no caduca —y no se dice por qué: un hold no se
 * anuncia—. Días hacia ARRIBA: lo borrado hoy dice «30 días», no «29», y lo que se
 * irá dentro de unas horas dice «1 día» hasta que el recolector pase.
 */
export function caducidad(n: Pick<TrashedNote, 'purge_at'>, ahora: Date): Caducidad {
  if (!n.purge_at) return { clave: 'trash.kept' };
  const quedan = Date.parse(n.purge_at) - ahora.getTime();
  if (Number.isNaN(quedan)) return { clave: 'trash.kept' };
  if (quedan <= 0) return { clave: 'trash.purgeSoon' };
  return { clave: 'trash.purgeIn', dias: Math.ceil(quedan / DIA_MS) };
}
