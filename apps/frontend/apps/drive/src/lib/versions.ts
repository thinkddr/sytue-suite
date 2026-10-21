/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Mapeo de una versión del historial de un fichero tal y como la emite el
 * servicio Go (snake_case) a camelCase para la UI. Mismo criterio que nodes.ts.
 */

/** Versión del historial tal y como la emite el servicio Go. */
export interface RawVersion {
  version: number;
  size_bytes?: number;
  content_type?: string;
  author_sub?: string;
  is_current?: boolean;
  created_at?: string;
}

/** Versión del historial en el vocabulario de la UI (camelCase). */
export interface FileVersion {
  version: number;
  sizeBytes: number;
  isCurrent: boolean;
  contentType?: string;
  author?: string;
  createdAt?: string;
}

export function toVersion(v: RawVersion): FileVersion {
  return {
    version: v.version,
    sizeBytes: v.size_bytes ?? 0,
    isCurrent: Boolean(v.is_current),
    ...(v.content_type ? { contentType: v.content_type } : {}),
    ...(v.author_sub ? { author: v.author_sub } : {}),
    ...(v.created_at ? { createdAt: v.created_at } : {}),
  };
}
