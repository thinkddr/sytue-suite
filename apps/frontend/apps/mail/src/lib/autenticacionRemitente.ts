/* SPDX-License-Identifier: Apache-2.0 */
import type { SenderAuth } from '@sytue/api-clients';

/**
 * Lo que el lector dice del remitente a partir de lo que comprobó el MX (SPF, DKIM,
 * DMARC). Tres salidas y no una tabla de siglas, porque quien lee no sabe qué es
 * DKIM: lo que necesita saber es si se puede fiar de que el correo es de quien dice.
 *
 * - `verificado`: DMARC pasa, es decir, el dominio del `From` respondió por el
 *   mensaje. Es lo único que lo prueba: un SPF o un DKIM sueltos pueden ser de
 *   otro dominio cualquiera, y llamar a eso «Verificado» sería mentir.
 * - `aviso`: DMARC falla, o sin DMARC SPF falla y nadie firmó con DKIM: el
 *   remitente puede estar suplantado.
 * - `null`: no hay base para decir ninguna de las dos (correo propio, importado,
 *   o un dominio sin política): no se pinta nada, como hasta ahora.
 *
 * Fyne no lo pinta aún: el lector nativo no recibe estos campos del sobre.
 */
export type ResumenRemitente = 'verificado' | 'aviso' | null;

export function resumenRemitente(auth: SenderAuth | undefined): ResumenRemitente {
  if (!auth) return null;
  if (auth.dmarc === 'pass') return 'verificado';
  if (auth.dmarc === 'fail') return 'aviso';
  const spfFalla = auth.spf === 'fail' || auth.spf === 'softfail';
  const dkimFalla = auth.dkim === undefined || auth.dkim === 'fail';
  return spfFalla && dkimFalla ? 'aviso' : null;
}

/** El detalle técnico para el `title`: «SPF pass · DKIM fail · DMARC pass». */
export function detalleRemitente(auth: SenderAuth): string {
  return (['spf', 'dkim', 'dmarc'] as const)
    .filter((k) => auth[k])
    .map((k) => `${k.toUpperCase()} ${auth[k]}`)
    .join(' · ');
}
