/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LAS FUNCIONES DE LA LIBRERÍA, BAJO DEMANDA.
 *
 * El motor armaba su registro (`buildRegistry`, ~245 KB con las quinientas funciones) en
 * el constructor, así que el editor de hojas no podía bajar sin ellas. Ahora el registro
 * se INSTALA: el índice del paquete lo instala al cargarse (lo de siempre, para quien lo
 * importa), y el editor importa `@sytue/formula/motor`, que no lo trae, y lo pide con
 * `cargarFunciones()` en un trozo aparte.
 *
 * Mientras no llega, el motor se construye con un registro de ESPERA que conoce todos
 * los nombres (`NOMBRES_REGISTRO`) y responde `#BUSY!` —el error de Excel para «aún
 * calculando»—: una fórmula sin funciones (`=A1*2`) se calcula ya, y una con funciones
 * no da nunca un valor equivocado ni un `#NAME?`; el editor la pinta vacía y vuelve a
 * construir el motor cuando llegan (`Engine.esperaFunciones`).
 */
import { Registry, type FnImpl } from './registry.js';
import { NOMBRES_REGISTRO } from './nombres-funciones.js';
import { ERR } from './values.js';

let instalado: (() => Registry) | undefined;
let cargando: Promise<void> | undefined;

/** Instala la fábrica del registro con todas las funciones. */
export function instalarFunciones(fabrica: () => Registry): void {
  instalado = fabrica;
}

/** Si las funciones ya están instaladas: un motor nuevo las tendrá todas. */
export function funcionesCargadas(): boolean {
  return instalado !== undefined;
}

/** Baja e instala las funciones. Idempotente; si el trozo no llega, el siguiente intento
 *  repite. */
export function cargarFunciones(): Promise<void> {
  if (instalado) return Promise.resolve();
  cargando ??= import('./functions/index.js').then(
    (m) => instalarFunciones(m.buildRegistry),
    (error: unknown) => {
      cargando = undefined;
      throw error;
    },
  );
  return cargando;
}

const enEspera: FnImpl = () => ERR.BUSY;
let registroDeEspera: Registry | undefined;

/** El registro para un motor nuevo: el de verdad si ya está instalado y, si no, el de
 *  espera (`pendiente`). */
export function registroParaMotor(): { registro: Registry; pendiente: boolean } {
  if (instalado) return { registro: instalado(), pendiente: false };
  registroDeEspera ??= new Registry().add(
    Object.fromEntries(NOMBRES_REGISTRO.map((n) => [n, enEspera])),
  );
  return { registro: registroDeEspera, pendiente: true };
}
