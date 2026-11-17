/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué acciones de Rooms no existen todavía para un mensaje cifrado de extremo a extremo.
 *
 * Las cuatro opciones del compositor (programar, encuesta, tarea, pedir acuse) y sus dos
 * respuestas (votar, confirmar) se apoyan en el servicio: es él quien guarda la cola de
 * programados, cuenta los votos y suma los acuses. Con el cifrado activo el servicio no
 * ve el mensaje, así que esas llamadas fallan (`postJson` las corta con
 * `MatrixSecurityError('unsupported')`) o, peor, publican una encuesta que nadie puede
 * votar. Decisión del dueño: NO esconderlas, sino dejarlas a la vista y desactivadas con
 * una línea que diga por qué; así se sabe que existen y que volverán.
 *
 * Una sola lista para la web (gemela de `apps/fyne/apps/rooms/cifrado.go`): cuando una de
 * estas acciones viaje por Matrix, se quita de aquí y vuelve a funcionar en todas partes.
 */

/** Acciones que dependen de que el servicio lea el mensaje. */
export type AccionCifrada =
  'programar' | 'encuesta' | 'tarea' | 'pedirAcuse' | 'votar' | 'confirmarAcuse';

const NO_DISPONIBLES_CIFRADO: ReadonlySet<AccionCifrada> = new Set<AccionCifrada>([
  'programar',
  'encuesta',
  'tarea',
  'pedirAcuse',
  'votar',
  'confirmarAcuse',
]);

/**
 * La web de Rooms publica SIEMPRE por Matrix (`chatApi.sendMessage` cifra con
 * `roomsMatrix().send` antes de llamar al BFF): no hay modo sin cifrar que elegir. Es una
 * constante y no un estado para que ChatApp la pase explícita y los tests prueben las dos
 * ramas con la misma función.
 */
export const CIFRADO_DE_EXTREMO_A_EXTREMO = true;

/** ¿Se puede ofrecer `accion` con el cifrado en ese estado? Sin cifrado, todo. */
export function disponibleConCifrado(accion: AccionCifrada, cifrado: boolean): boolean {
  return !cifrado || !NO_DISPONIBLES_CIFRADO.has(accion);
}
