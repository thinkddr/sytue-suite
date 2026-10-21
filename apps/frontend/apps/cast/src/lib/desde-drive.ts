/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Subir a Cast» desde Sytue Drive: Drive manda a `/cast/upload?drive=<id>` y esta
 * página enseña el fichero, deja cambiar título y visibilidad y pide al servicio
 * la copia. Aquí va lo que se decide sin pintar: qué fichero pide la URL y qué
 * se le dice a quien sube según lo que contestó el servicio.
 */
import { FalloDelServicio } from './fallo.js';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * El fichero de Drive que pide la URL: `null` si no pide ninguno (la subida de
 * siempre) y `''` si lo pide pero no es un identificador (el estado vacío, no una
 * petición al servicio con cualquier cosa).
 */
export function idDeDrive(busqueda: string): string | null {
  const p = new URLSearchParams(busqueda);
  if (!p.has('drive')) return null;
  const id = (p.get('drive') ?? '').trim();
  return UUID.test(id) ? id.toLowerCase() : '';
}

/**
 * Por qué no se puede subir, en el vocabulario de la pantalla. Cada uno tiene su
 * texto (`upload.drive.error.<motivo>`), salvo `offline` y `sinPermiso`, que son
 * estados de página enteros.
 */
export type MotivoDeDrive =
  'offline' | 'sinPermiso' | 'noVideo' | 'grande' | 'cuota' | 'drive' | 'error';

export function motivoDeDrive(e: unknown, enLinea: boolean = true): MotivoDeDrive {
  if (!enLinea) return 'offline';
  if (e instanceof FalloDelServicio) {
    switch (e.status) {
      // «No existe» y «no puedes leerlo» se dicen igual: distinguirlos diría a quien
      // no tiene permiso que el fichero existe.
      case 403:
      case 404:
        return 'sinPermiso';
      case 413:
        return 'grande';
      case 415:
        return 'noVideo';
      case 507:
        return 'cuota';
      case 503:
        return 'drive';
    }
    return 'error';
  }
  // `fetch` rechaza con TypeError cuando no llega a haber respuesta.
  if (e instanceof TypeError) return 'offline';
  return 'error';
}

/** Bytes libres que el servicio dijo al rechazar por cuota, si los dijo. */
export function libresDe(e: unknown): number | null {
  if (!(e instanceof FalloDelServicio)) return null;
  const d = e.detalles['available_bytes'];
  return typeof d === 'number' ? d : null;
}

/** Si tiene sentido reintentar lo mismo: solo cuando el fallo no es del fichero. */
export function reintentable(m: MotivoDeDrive): boolean {
  return m === 'drive' || m === 'error' || m === 'offline';
}
