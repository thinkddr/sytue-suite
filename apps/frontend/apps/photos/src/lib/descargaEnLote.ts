/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

// Política de la app para acotar y espaciar las peticiones. No garantiza que el
// navegador las permita ni que los archivos terminen de guardarse.
export const TOPE_DESCARGA = 25;
export const PAUSA_DESCARGA = 600;

export interface PlanDeDescarga {
  /** IDs cuyas peticiones se incluyen en este lote, en el orden de selección. */
  ahora: string[];
  /** IDs válidos que quedan fuera del lote. */
  fuera: number;
}

/** Ignora IDs vacíos y conserva el orden sin alterar la selección recibida. */
export function planDeDescarga(ids: readonly string[]): PlanDeDescarga {
  const limpios = ids.filter((id) => id.trim() !== '');
  return {
    ahora: limpios.slice(0, TOPE_DESCARGA),
    fuera: Math.max(0, limpios.length - TOPE_DESCARGA),
  };
}

export interface RelojDeDescarga {
  programar: (accion: () => void, espera: number) => number;
  cancelar: (id: number) => void;
}

/** Primera petición síncrona; el resto se puede cancelar incluso si ya venció. */
export function programarDescarga(
  ids: readonly string[],
  iniciar: (id: string) => void,
  reloj: RelojDeDescarga,
): () => void {
  let cancelada = false;
  const primera = ids[0];
  if (primera !== undefined) iniciar(primera);
  const temporizadores = ids.slice(1).map((id, i) =>
    reloj.programar(
      () => {
        if (!cancelada) iniciar(id);
      },
      (i + 1) * PAUSA_DESCARGA,
    ),
  );
  return () => {
    cancelada = true;
    temporizadores.forEach((id) => reloj.cancelar(id));
  };
}
