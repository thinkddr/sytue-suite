/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El cifrado de bloques de Matrix (`Attachment`, `EncryptedAttachment` de
 * `@matrix-org/matrix-sdk-crypto-wasm`), bajado la primera vez que se cifra o se
 * descifra un bloque.
 *
 * Su JS (~244 KB sin comprimir, sin contar el wasm) llegaba al arranque de todas las
 * apps de la bóveda por `storage-files` ← `storage-repository` ← `vault/pq-vault.ts`,
 * aunque solo lo usan los ficheros: subir, bajar o verificar un bloque. Ahora vive en
 * su propio trozo y lo pide quien cifra o descifra, con `import()`, justo donde antes
 * esperaba a `initAsync()`. Las comprobaciones de alrededor (tamaños, resúmenes,
 * borrado de los búferes) no cambian: solo cambia de dónde sale el módulo.
 */
import type * as MatrixCryptoWasm from '@matrix-org/matrix-sdk-crypto-wasm';

export type MatrixAttachmentWasm = Pick<
  typeof MatrixCryptoWasm,
  'Attachment' | 'EncryptedAttachment'
>;

let cargando: Promise<MatrixAttachmentWasm> | undefined;

/**
 * El módulo ya inicializado (`initAsync`). Idempotente: todas las llamadas comparten
 * la misma carga; si falla (el trozo no llegó), la siguiente lo vuelve a pedir.
 */
export function matrixAttachments(): Promise<MatrixAttachmentWasm> {
  cargando ??= import('@matrix-org/matrix-sdk-crypto-wasm')
    .then(async (modulo) => {
      await modulo.initAsync();
      return modulo;
    })
    .catch((error: unknown) => {
      cargando = undefined;
      throw error;
    });
  return cargando;
}
