/* SPDX-License-Identifier: Apache-2.0 */
// La puerta del Drive de la bóveda vive en `@sytue/offline/vault/drive`, que
// comparte con las apps que guardan en el Drive de la persona (Photos).
export {
  createPrivateDriveGate,
  type PrivateDriveSession,
  type PrivateDriveVaultHost,
} from '@sytue/offline/vault/drive';
