/* SPDX-License-Identifier: Apache-2.0 */
import { type SuiteMatrix, MatrixSecurityError } from '@sytue/offline/matrix';
// Rooms vive en Matrix: el valor del SDK, de su propio subpath (el índice lo da solo
// como tipo para que las apps de la bóveda no lo bajen).
export { SuiteMatrix as RoomsMatrix } from '@sytue/offline/matrix/client';
export {
  safeAttachmentType,
  validCallContent,
  type MatrixRecoveryUI,
  type MatrixDevice,
  type MatrixCallEventType,
  type MatrixCallKeyContent,
} from '@sytue/offline/matrix';

let active: SuiteMatrix | null = null;
export function setRoomsMatrix(client: SuiteMatrix | null): void {
  active = client;
}
export function roomsMatrix(): SuiteMatrix {
  if (!active || active.isClosed) throw new MatrixSecurityError('device');
  return active;
}
