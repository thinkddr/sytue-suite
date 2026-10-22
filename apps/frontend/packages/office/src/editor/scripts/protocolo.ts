/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Protocolo de los scripts EN DOCS: el protocolo es el común (`@sytue/scripts`); aquí
 * solo vive la superficie `sytue.v1` de Docus, Calcus y Diapos, de la que salen la lista
 * cerrada de operaciones que acepta el anfitrión y la API que ve el script. Una sola
 * tabla para las dos cosas: no pueden separarse.
 */
import { leerMensaje as leerMensajeComun } from '@sytue/scripts';
import type { MensajeDeSandbox as MensajeComun } from '@sytue/scripts';

export { LIMITES, sanearDato } from '@sytue/scripts';
export type { Escalar, Respuesta } from '@sytue/scripts';

export const SUPERFICIE = [
  { op: 'run', min: 1, max: 2 },
  { op: 'select', min: 2, max: 2 },
  { op: 'read.cell', min: 2, max: 2 },
  { op: 'read.range', min: 2, max: 2 },
  { op: 'read.text', min: 0, max: 0 },
  { op: 'read.slides', min: 0, max: 0 },
  { op: 'read.selection', min: 0, max: 0 },
  { op: 'read.title', min: 0, max: 0 },
] as const;
export type Operacion = (typeof SUPERFICIE)[number]['op'];
export const OPERACIONES: readonly Operacion[] = SUPERFICIE.map((s) => s.op);

/** Del sandbox al anfitrión. */
export type MensajeDeSandbox = MensajeComun<Operacion>;

/** Valida un mensaje del sandbox contra las operaciones de Docs. */
export const leerMensaje = (crudo: unknown) => leerMensajeComun(crudo, OPERACIONES);
