/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Resolución de la prueba de trabajo ALTCHA en el navegador (estándar abierto: SHA-256 con
 * Web Crypto, sin cookies ni terceros). Es el mismo contrato que `platform/pkg/altcha`:
 * se busca el número `n` ≤ `maxnumber` tal que SHA-256(salt + n) en hexadecimal sea el
 * `challenge`, y se devuelve la carga que el servicio comprueba (y acepta una sola vez).
 *
 * Vive aquí porque lo usan los formularios públicos de Sytue Bases y la denuncia sin
 * sesión de Sytue Cast: una sola resolución para un solo contrato.
 */
export interface RetoAltcha {
  algorithm: string;
  challenge: string;
  maxnumber: number;
  salt: string;
  signature: string;
}

const hex = (b: ArrayBuffer): string =>
  [...new Uint8Array(b)].map((x) => x.toString(16).padStart(2, '0')).join('');

/** Resuelve el reto; devuelve la carga en base64 o `null` si no tiene solución (reto roto). */
export async function resolverReto(
  reto: RetoAltcha,
  digest = (d: BufferSource) => crypto.subtle.digest('SHA-256', d),
): Promise<string | null> {
  if (reto.algorithm !== 'SHA-256') return null;
  const enc = new TextEncoder();
  for (let n = 0; n <= reto.maxnumber; n++) {
    if (hex(await digest(enc.encode(reto.salt + n))) === reto.challenge) {
      const carga = {
        algorithm: reto.algorithm,
        challenge: reto.challenge,
        number: n,
        salt: reto.salt,
        signature: reto.signature,
      };
      return btoa(JSON.stringify(carga));
    }
  }
  return null;
}
