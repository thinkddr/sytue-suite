/* SPDX-License-Identifier: Apache-2.0 */
export {
  PqVault,
  PqVaultError,
  PqVaultDisabledError,
  fetchPqVaultBootstrap,
  readPqVaultCopy,
  cancelPqVaultCopy,
  type PqVaultBootstrap,
  type PqVaultCopyMarker,
  type PqVaultCopyOptions,
  type PqVaultExpect,
  type PqVaultBridgePort,
  type PqVaultOpenOptions,
  type PqVaultReason,
  type PqVaultStateResponse,
} from './pq-vault.js';
export {
  pqVaultProfile,
  bridgeHandoffKey,
  takeHandoff,
  unwrapVaultInBridge,
  wrapVaultInBridge,
  type PqVaultSecret,
  type PqVaultWrapper,
} from '../matrix/unlock-bridge.js';
