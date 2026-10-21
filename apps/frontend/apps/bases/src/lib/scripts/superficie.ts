/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La superficie `sytue.v1` de Sytue Bases: la lista CORTA y cerrada de lo que un script
 * puede pedir. De esta tabla salen las dos cosas —la API que ve el script dentro del
 * sandbox y las operaciones que acepta el anfitrión—, así que no pueden separarse.
 *
 * Es el espejo de `domain.OperacionesDeScript` (services/bases): `superficie.test.ts` lee
 * el fichero Go y falla si una lista tiene algo que la otra no, o si no coinciden en cuál
 * escribe. Fuera quedan, a propósito, el esquema, los permisos, compartir, conectores,
 * formularios, adjuntos, comentarios, exportar e importar.
 */
import type { OperacionDeSuperficie } from '@sytue/scripts';

export const SUPERFICIE = [
  { op: 'tables.list', min: 0, max: 0, escribe: false },
  { op: 'fields.list', min: 1, max: 1, escribe: false },
  { op: 'views.list', min: 1, max: 1, escribe: false },
  // (tabla, { view?, cursor?, limit? })
  { op: 'records.list', min: 1, max: 2, json: true, escribe: false },
  { op: 'records.get', min: 1, max: 1, escribe: false },
  // (tabla, { <campo>: valor })  y  (registro, { <campo>: valor })
  { op: 'records.create', min: 2, max: 2, json: true, escribe: true },
  { op: 'records.update', min: 2, max: 2, json: true, escribe: true },
  { op: 'records.delete', min: 1, max: 1, escribe: true },
] as const satisfies readonly (OperacionDeSuperficie & { escribe: boolean })[];

export type Operacion = (typeof SUPERFICIE)[number]['op'];
export const OPERACIONES: readonly Operacion[] = SUPERFICIE.map((s) => s.op);

const QUE_ESCRIBE: ReadonlySet<string> = new Set(
  SUPERFICIE.filter((s) => s.escribe).map((s) => s.op),
);
export const esOperacion = (op: string): op is Operacion =>
  (OPERACIONES as readonly string[]).includes(op);
export const escribe = (op: string): boolean => QUE_ESCRIBE.has(op);
