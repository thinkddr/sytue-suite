/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * «Añadir a favoritas» / «Quitar de favoritas» desde la barra de selección. La
 * misma regla vive en Fyne (`planFavoritas` y `conFavoritas` en
 * apps/fyne/apps/photos/app_galeria.go).
 */

interface ConFavorita {
  id: string;
  favorite?: boolean;
}

/**
 * Qué hace el corazón de la barra con lo marcado. Si TODAS son ya favoritas, las
 * quita; si falta alguna, las marca — como el corazón de una sola foto, que dice
 * lo que pasará al pulsarlo. Solo se piden las que cambian: marcar doce donde
 * diez ya lo eran son dos llamadas, no doce.
 */
export function planFavoritas<F extends ConFavorita>(
  fotos: readonly F[],
  seleccion: ReadonlySet<string>,
): { valor: boolean; ids: string[] } {
  const marcadas = fotos.filter((f) => seleccion.has(f.id));
  // Las marcadas que ya no están cargadas (la selección sobrevive a una búsqueda
  // nueva): su corazón no se conoce, así que no se puede decir que TODAS sean
  // favoritas. Se marcan, que no cambia nada en las que ya lo eran.
  const cargadas = new Set(marcadas.map((f) => f.id));
  const fuera = [...seleccion].filter((id) => !cargadas.has(id));
  const valor =
    fuera.length > 0 || marcadas.length === 0 || !marcadas.every((f) => f.favorite === true);
  const ids = marcadas.filter((f) => (f.favorite === true) !== valor).map((f) => f.id);
  return { valor, ids: valor ? [...ids, ...fuera] : ids };
}

/**
 * La lista tras el lote, con SOLO las que el servicio aceptó. En «Favoritas»,
 * desmarcar las saca de la vista; en las demás, cambia su corazón.
 */
export function conFavoritas<F extends ConFavorita>(
  fotos: readonly F[],
  hechas: ReadonlySet<string>,
  valor: boolean,
  soloFavoritas: boolean,
): F[] {
  if (soloFavoritas && !valor) return fotos.filter((f) => !hechas.has(f.id));
  return fotos.map((f) => (hechas.has(f.id) ? { ...f, favorite: valor } : f));
}

/**
 * Un lote a la vez: mientras el anterior no ha vuelto, otra pulsación no lanza
 * otro. Con un doble clic el plan se calculaba dos veces y cada foto se pedía
 * dos veces (Fyne guarda lo mismo con `marcandoFavoritas`). El cerrojo es un
 * `useRef` y no un estado: el estado no cambia hasta el siguiente pintado, y
 * dos clics seguidos lo leerían igual. Devuelve si llegó a lanzarlo.
 */
export async function unLoteALaVez(
  cerrojo: { current: boolean },
  lote: () => Promise<void>,
): Promise<boolean> {
  if (cerrojo.current) return false;
  cerrojo.current = true;
  try {
    await lote();
  } finally {
    cerrojo.current = false;
  }
  return true;
}
