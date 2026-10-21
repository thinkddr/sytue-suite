/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Actividad del documento (auditoría, solo el propietario): una línea por grupo de
 * operaciones, sin el contenido de lo escrito. Mismo texto que las apps nativas
 * (apps/nucleo/api/docs_auditoria.go, `Descripcion`).
 */
import type { AuditEntry } from '../types';

/** Cuántas rutas se nombran en la línea; el resto, «…». */
const MAX_RUTAS = 3;

/**
 * Una ruta de parche en pocas palabras: la celda (`B2`), la caja (`boxes·html`) o la
 * propiedad de la hoja o la diapositiva (`freeze`). Sin el id de la hoja, que es un UUID.
 */
export function rutaCorta(ruta: string): string {
  const partes = ruta.split('/');
  if (partes[0] !== 'sheets' && partes[0] !== 'slides') return ruta;
  const resto = partes.slice(2); // sin «sheets/<id>»
  if (resto[0] === 'cells' && resto[1]) return resto[1];
  if (resto[0] === 'boxes') return ['boxes', ...resto.slice(2)].join('·');
  return resto.join('/') || partes[0];
}

/** Qué tocó el grupo: las celdas o cajas de sus parches, o la clave. */
export function queToco(e: Pick<AuditEntry, 'paths' | 'key' | 'kind'>): string {
  const rutas = [...new Set((e.paths ?? []).map(rutaCorta))];
  if (rutas.length > 0)
    return rutas.slice(0, MAX_RUTAS).join(', ') + (rutas.length > MAX_RUTAS ? ', …' : '');
  return e.key || e.kind;
}

/** Nombre de quien hizo el cambio: el que resolvió el BFF o, sin él, un resumen del sub. */
export function autorDeActividad(sub: string, nombres: Readonly<Record<string, string>>): string {
  return nombres[sub] || sub.slice(0, 8);
}

/** La línea completa: «quien — qué ×n · cuándo». */
export function lineaDeActividad(
  e: AuditEntry,
  cuando: string,
  nombres: Readonly<Record<string, string>> = {},
): string {
  return `${autorDeActividad(e.author_sub, nombres)} — ${queToco(e)} ×${e.count} · ${cuando}`;
}

/** Tipos de operación que enseña el filtro, en el orden del selector. */
export const TIPOS_ACTIVIDAD = ['insert', 'delete', 'set', 'patch'] as const;

/** Las entradas de la persona y el tipo elegidos ('' = todas). */
export function filtrarActividad(
  entradas: readonly AuditEntry[],
  persona: string,
  tipo: string,
): AuditEntry[] {
  return entradas.filter(
    (e) => (persona === '' || e.author_sub === persona) && (tipo === '' || e.kind === tipo),
  );
}

/** Las personas distintas de las entradas cargadas, por orden de aparición. */
export function personasDeActividad(entradas: readonly AuditEntry[]): string[] {
  return [...new Set(entradas.map((e) => e.author_sub))];
}
