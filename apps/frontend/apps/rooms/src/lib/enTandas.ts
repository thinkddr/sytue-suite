/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Hacer N peticiones de pocas en pocas, sin que una que falle pare a las demás.
 *
 * «Marcar todo como leído» mandaba un `markRead` por canal pendiente, todos a la vez:
 * con cien canales eran cien peticiones simultáneas contra un servicio que limita por
 * persona, y las que se pasaban del tope volvían con 429 en silencio. La app de
 * escritorio ya las mandaba de una en una; aquí van de cuatro en cuatro, que es rápido
 * y no choca con el tope.
 */

/** Cuántas peticiones de «leído» van a la vez. */
export const TANDA_DE_LEIDOS = 4;

/**
 * Llama a `hacer` con cada elemento, como mucho `tanda` a la vez, y espera a todas. Un
 * fallo no para a las demás (`allSettled`); devuelve cuántas salieron bien.
 */
export async function enTandas<T>(
  elementos: readonly T[],
  tanda: number,
  hacer: (x: T) => Promise<unknown>,
): Promise<number> {
  let siguiente = 0;
  let bien = 0;
  const trabajador = async () => {
    while (siguiente < elementos.length) {
      const x = elementos[siguiente++]!;
      try {
        await hacer(x);
        bien++;
      } catch {
        // Uno que falle no para a los demás: el siguiente recuento lo devuelve.
      }
    }
  };
  await Promise.all(
    Array.from({ length: Math.max(1, Math.min(tanda, elementos.length)) }, trabajador),
  );
  return bien;
}
