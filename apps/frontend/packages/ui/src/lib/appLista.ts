/* SPDX-License-Identifier: Apache-2.0 */
/**
 * «La app ya se puede usar»: la marca `sytue:app-ready` del Performance Timeline.
 *
 * El LCP de una app con bóveda medía el texto de la puerta del cifrado, no la app
 * (roadmap, «Rendimiento y estética de la web», 2026-11-24). Esta marca la pone la
 * propia app cuando pinta sus datos de verdad —la lista de notas, los eventos del
 * mes— y es la que se presupuesta (`.lighthouserc.apps.json`, `sytue-app-ready`).
 *
 * Sirve también de reloj a la puerta de la bóveda en segundo plano (`PqVaultGate`
 * con `background`): una app que no guarda nada en la bóveda no comparte el ancho
 * de banda de su primera carga con los 2 MB del núcleo; la bóveda empieza a abrirse
 * cuando la app ya está lista.
 *
 * Una sola marca por documento, la primera: se mira el propio Performance Timeline y
 * no una variable del módulo, porque dos trozos de JS con su copia de este módulo
 * tienen que estar de acuerdo.
 */

export const MARCA_APP_LISTA = 'sytue:app-ready';

function yaMarcada(): boolean {
  try {
    return performance.getEntriesByName(MARCA_APP_LISTA, 'mark').length > 0;
  } catch {
    return false;
  }
}

/** Pone la marca `sytue:app-ready` (solo la primera vez) y avisa a quien espera. */
export function marcarAppLista(): void {
  if (typeof performance === 'undefined' || yaMarcada()) return;
  try {
    performance.mark(MARCA_APP_LISTA);
  } catch {
    return;
  }
  try {
    globalThis.dispatchEvent?.(new Event(MARCA_APP_LISTA));
  } catch {
    // Sin `window` (SSR, un worker): nadie espera.
  }
}

/**
 * Se resuelve cuando la app se marca como lista o, si no lo hace nunca, a los
 * `maximoMs` (una app que no pone la marca no deja la bóveda sin abrir). Con la
 * señal abortada no se resuelve nunca: quien esperaba ya no está.
 */
export function cuandoAppLista(signal?: AbortSignal, maximoMs = 5000): Promise<void> {
  return new Promise<void>((resolve) => {
    if (signal?.aborted) return;
    if (typeof performance === 'undefined' || yaMarcada()) {
      resolve();
      return;
    }
    const listo = (): void => {
      limpiar();
      resolve();
    };
    const abortar = (): void => limpiar();
    const reloj = setTimeout(listo, maximoMs);
    function limpiar(): void {
      clearTimeout(reloj);
      globalThis.removeEventListener?.(MARCA_APP_LISTA, listo);
      signal?.removeEventListener('abort', abortar);
    }
    globalThis.addEventListener?.(MARCA_APP_LISTA, listo, { once: true });
    signal?.addEventListener('abort', abortar, { once: true });
  });
}
