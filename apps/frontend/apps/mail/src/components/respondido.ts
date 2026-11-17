/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Las respuestas que esperan a salir para marcar su original como respondido.
 *
 * Contestar desde Sytue pone `$answered` (`\Answered` en IMAP) en el correo al que
 * se contesta, que es lo que Thunderbird o el iPhone leen para pintar su flecha. Pero
 * solo cuando el envío SALE: durante la ventana de deshacer el correo sigue retenido
 * en la cola y puede no salir nunca. Esto lleva el plazo de cada envío retenido y dice
 * qué hay que quitar si se deshace después de que el plazo se cumpliera.
 *
 * El reloj se inyecta para que las pruebas no esperen diez segundos de verdad.
 */

/** Programa `f` dentro de `ms` y devuelve con qué pararlo. */
export type Esperar = (ms: number, f: () => void) => () => void;

const esperarDeVerdad: Esperar = (ms, f) => {
  const reloj = setTimeout(f, ms);
  return () => clearTimeout(reloj);
};

interface Retenida {
  id: string;
  puesta: boolean;
  parar: () => void;
}

export interface RespuestasRetenidas {
  /**
   * El envío `outboundId` contesta a `id`: `poner` se llama cuando se cumplen sus
   * `segundos` de ventana (o ya, si no hay ventana).
   */
  retener: (
    outboundId: string | undefined,
    id: string,
    segundos: number,
    poner: () => void,
  ) => void;
  /**
   * Se ha intentado deshacer `outboundId`. Si la cola lo canceló, el plazo deja de
   * correr y, si la marca ya se había puesto, devuelve el id al que hay que quitársela.
   * Si no lo canceló (ya salió), la marca es cierta y se deja.
   */
  anular: (outboundId: string, cancelado: boolean) => string | null;
  /** Suelta todos los plazos (al desmontar). */
  parar: () => void;
}

export function respuestasRetenidas(esperar: Esperar = esperarDeVerdad): RespuestasRetenidas {
  const retenidas = new Map<string, Retenida>();
  return {
    retener(outboundId, id, segundos, poner) {
      if (!outboundId || segundos <= 0) {
        poner();
        return;
      }
      const r: Retenida = { id, puesta: false, parar: () => {} };
      r.parar = esperar(segundos * 1000, () => {
        r.puesta = true;
        poner();
      });
      retenidas.set(outboundId, r);
    },
    anular(outboundId, cancelado) {
      const r = retenidas.get(outboundId);
      if (!r) return null;
      retenidas.delete(outboundId);
      if (!cancelado) return null;
      r.parar();
      // El plazo pudo cumplirse un instante antes de que la cola soltara el envío.
      return r.puesta ? r.id : null;
    },
    parar() {
      for (const r of retenidas.values()) r.parar();
      retenidas.clear();
    },
  };
}
