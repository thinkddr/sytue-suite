/* SPDX-License-Identifier: Apache-2.0 */
/**
 * `@sytue/shell/vault`: las puertas de las apps de la bóveda (ADR 0086) sin Matrix en
 * el JS inicial.
 *
 * `@sytue/shell/matrix` exporta además la puerta de dispositivos de Matrix, y con ella
 * matrix-js-sdk entero. Desde aquí `MatrixStorageGate` y `MatrixSourceGate` solo bajan
 * su rama de Matrix cuando la pintan (`matrix/matrix-lazy.tsx`). Rooms, que vive en
 * Matrix, sigue con `@sytue/shell/matrix`.
 */
export {
  MatrixSourceGate,
  MatrixStorageGate,
  type MatrixSourceGateHost,
  type MatrixLegacyResolver,
} from './matrix/MatrixSourceGate.js';
export type { MatrixGateHost, MatrixGateMessage } from './matrix/MatrixDeviceGate.js';
export { PqVaultGate, type PqVaultGateProps } from './matrix/PqVaultGate.js';
export { SourceReleaseNotice } from './matrix/SourceReleaseNotice.js';
