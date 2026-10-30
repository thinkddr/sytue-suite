/* SPDX-License-Identifier: Apache-2.0 */
import { type SuiteMatrix, MatrixSecurityError } from '@sytue/offline/matrix';
export {
  SuiteMatrix as RoomsMatrix,
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
