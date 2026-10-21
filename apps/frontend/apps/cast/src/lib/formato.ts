/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Formatos propios del vídeo que Intl no da hechos.
 */

/**
 * Duración de reproductor: `0:07`, `4:05`, `1:02:03`. Es la notación de cualquier
 * reproductor y no depende del idioma (Intl.DurationFormat aún no está en todos
 * los navegadores de la suite, y escribiría «4 min, 5 s» donde se espera `4:05`).
 */
export function duracion(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || !Number.isFinite(ms) || ms < 0) return '0:00';
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = String(s).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}
