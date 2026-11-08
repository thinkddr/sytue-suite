/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Cuándo la galería enseña «Ninguna foto coincide con…» y qué ofrece. La misma
 * regla vive en Fyne (`sinResultados` en apps/fyne/apps/photos/app.go).
 */
export function sinResultados(e: {
  texto: string;
  cargando: boolean;
  /**
   * La búsqueda escrita que aún espera al temporizador: no se ha pedido, así que
   * decir que no encuentra nada sería falso. Cuenta como cargando.
   */
  pendiente: boolean;
  fotos: number;
  /** Hubo un fallo: entonces se dice el fallo, no que no hay nada. */
  error: boolean;
  /** Los términos con los que ya se amplió; con alguno, ampliar no se vuelve a ofrecer. */
  tambien: number;
}): { consulta: string; ampliar: boolean } | null {
  const consulta = e.texto.trim();
  if (consulta === '' || e.cargando || e.pendiente || e.fotos > 0 || e.error) return null;
  return { consulta, ampliar: e.tambien === 0 };
}
