/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Cuánto le queda a una foto en la papelera. La fecha de purga la pone el servicio
 * (`purgeAt`), que es quien purga: aquí solo se cuenta. La misma cuenta vive en Fyne
 * (`Photo.DiasParaPurga` en apps/nucleo/api/photos.go), para que la web y el
 * escritorio no prometan días distintos para la misma foto.
 *
 * Hacia arriba: a 30 horas de la purga quedan «2 días», no 1. Y 0 si ya le toca —el
 * servicio purga por lotes y puede llegar un poco tarde—. `null` es que no hay fecha:
 * la organización no caduca la papelera, y entonces no se promete ninguna.
 */
export function diasParaPurga(purgeAt: string | undefined, ahora: number): number | null {
  if (!purgeAt) return null;
  const cuando = Date.parse(purgeAt);
  if (Number.isNaN(cuando)) return null;
  const falta = cuando - ahora;
  return falta <= 0 ? 0 : Math.ceil(falta / 86_400_000);
}
