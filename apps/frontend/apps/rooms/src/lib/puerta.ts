/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La sala de espera desde el lado de quien llama: después de llamar a la puerta se
 * vuelve a intentar entrar cada poco, hasta que el anfitrión deja pasar o se agota la
 * espera. Es lo que hace Fyne (`PausaPuerta`, `IntentosPuerta` en
 * apps/fyne/apps/rooms/videollamada.go): sin esto, quien llamaba tenía que volver a
 * abrir el enlace después de que le abrieran, sin saber cuándo. La reunión de un evento
 * recibe así a los invitados de otra organización que no traen la clave.
 */

/** Lo mismo que Fyne: cada 8 s durante unos cinco minutos. */
export const PAUSA_PUERTA_MS = 8_000;
export const INTENTOS_PUERTA = 38;

export interface EsperaPuerta<T> {
  /** Intenta entrar. Si la puerta sigue echada, debe fallar con un error que `cerrada` reconozca. */
  entrar: () => Promise<T>;
  /** ¿Es este error la puerta echada (`room_locked`)? Cualquier otro corta la espera. */
  cerrada: (err: unknown) => boolean;
  /** Espera entre intentos; en las pruebas no espera de verdad. */
  esperar?: (ms: number) => Promise<void>;
  /** ¿Se ha cancelado (se cerró la vista, se pidió otra reunión)? Se mira antes de cada intento. */
  cancelado?: () => boolean;
  pausaMs?: number;
  intentos?: number;
}

/**
 * Vuelve a intentar entrar tras llamar a la puerta. Devuelve lo que devuelva `entrar`
 * cuando la abren, `null` si se agota la espera o se cancela, y propaga cualquier
 * error que no sea la puerta echada (una reunión cerrada, la red caída): no tiene
 * sentido seguir esperando a una puerta que ya no está.
 */
export async function esperarALaPuerta<T>(o: EsperaPuerta<T>): Promise<T | null> {
  const esperar = o.esperar ?? ((ms: number) => new Promise<void>((r) => window.setTimeout(r, ms)));
  const intentos = o.intentos ?? INTENTOS_PUERTA;
  for (let i = 0; i < intentos; i++) {
    await esperar(o.pausaMs ?? PAUSA_PUERTA_MS);
    if (o.cancelado?.()) return null;
    try {
      return await o.entrar();
    } catch (err) {
      if (!o.cerrada(err)) throw err;
    }
  }
  return null;
}
