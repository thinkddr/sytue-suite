/* SPDX-License-Identifier: Apache-2.0 */
export {
  SuiteMatrix,
  safeAttachmentType,
  validCallContent,
  type MatrixRecoveryUI,
  type MatrixDevice,
  type MatrixSessionAdapter,
  type MailPrivatePorts,
  type MatrixCallEventType,
  type MatrixCallKeyContent,
} from './client.js';
export * from './types.js';
export {
  MatrixCustodyError,
  type MatrixCustodyState,
  type MatrixKeyCustody,
  type MatrixUnlockMethod,
} from './custody.js';
export type { UnlockBridgeError } from './unlock-bridge.js';
export * from './storage.js';
export { StorageRepository, type StorageRecordTransport } from './storage-repository.js';

export {
  StorageFiles,
  maxStorageFileBytes,
  type StorageFileRequest,
  type StorageFileSink,
  type StorageBlobPort,
} from './storage-files.js';

export { StorageFileUploads, type StorageFileSource } from './storage-file-upload.js';
export { StorageStagedFiles, type StorageStagedFile } from './storage-staged-files.js';

export { PrivateDriveRepository, PrivateDriveError } from './private-drive.js';

export type { DriveMigrationSource } from './private-drive-migration.js';

// Módulos de la línea de sytue-native que conviven con la red de la rama:
// bloques de almacenamiento y bóveda SSSS con sesiones atestadas.
export * from './storage-blocks.js';
export * from './ssss-vault.js';

export { MatrixSessionProvenanceMissing } from './session-provenance.js';
