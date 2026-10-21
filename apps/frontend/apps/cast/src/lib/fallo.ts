/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * De qué tipo es un fallo, para pintar el estado que toca (CAST-ENCARGO §10: seis
 * estados por página). Sin conexión no es lo mismo que «el servicio falló», y «no
 * existe» y «no puedes verlo» se pintan igual a propósito: distinguirlos diría a
 * quien no tiene permiso que el vídeo existe.
 */

export type TipoDeFallo = 'offline' | 'sinPermiso' | 'error';

export class FalloDelServicio extends Error {
  constructor(
    readonly status: number,
    readonly code: string = '',
    readonly detalles: Record<string, unknown> = {},
  ) {
    super(`Sytue Cast respondió ${status}${code ? ` (${code})` : ''}`);
    this.name = 'FalloDelServicio';
  }
}

export function tipoDeFallo(e: unknown, enLinea: boolean = true): TipoDeFallo {
  if (!enLinea) return 'offline';
  if (e instanceof FalloDelServicio) {
    if (e.status === 403 || e.status === 404) return 'sinPermiso';
    return 'error';
  }
  // `fetch` rechaza con TypeError cuando no llega a haber respuesta.
  if (e instanceof TypeError) return 'offline';
  return 'error';
}
