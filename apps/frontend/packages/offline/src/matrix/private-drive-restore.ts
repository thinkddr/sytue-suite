/* SPDX-License-Identifier: Apache-2.0 */
/** Restoring an archive publishes a new version through the shared encrypted journal. */
import { maxLegacySourceBytes } from '@sytue/config/matrix';
import {
  encodeStorageJSON,
  storageJSONBytes,
  storageScope,
  storageIdentifier,
  storageReference,
  type StorageObjectVersion,
  type StoragePublicationTarget,
} from './storage.js';
import { fields, digest, invalid } from './storage-file-codec.js';
import {
  bodyPointer,
  driveObject,
  parseSummary,
  patchDriveJSON,
  verifyProjection,
  type DriveBodyPointer,
  type DriveSummary,
} from './private-drive-codec.js';
import {
  decodeDriveEntryCheckpoints,
  publishDriveEntry,
  saveDriveWriteIntent,
  type DriveEntryHost,
  type DriveMetadataRead,
  type DriveWriteIntent,
} from './private-drive-metadata.js';

export interface DriveRestoreRequest {
  id: string;
  version: number;
}
export interface DriveVersionSource {
  item: DriveSummary;
  binary: DriveBodyPointer;
  checksum: string;
}
export interface DriveRestoreHost extends DriveEntryHost {
  capture: (
    request: DriveRestoreRequest,
    signal: AbortSignal,
  ) => Promise<
    DriveMetadataRead & {
      base: StorageObjectVersion;
      source: DriveVersionSource;
      restored: StorageObjectVersion;
    }
  >;
  source: (version: StorageObjectVersion, signal: AbortSignal) => Promise<DriveVersionSource>;
  validate: (
    item: DriveSummary,
    base: DriveWriteIntent['base'],
    revision: string,
    signal: AbortSignal,
  ) => Promise<void>;
}
interface RestoreIntent extends DriveWriteIntent {
  v: 1;
  operation: 'restore_version';
  input: string;
  request: DriveRestoreRequest;
  restored: { object_id: string; reference: StorageObjectVersion['reference'] };
}
const encoder = new TextEncoder(),
  equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
function patch(source: DriveVersionSource, prior: DriveSummary, now: string, subject: string) {
  if (prior.version < 1 || prior.version >= 0x7fffffff) invalid();
  return {
    size_bytes: source.item.size_bytes,
    content_type: source.item.content_type,
    checksum: source.checksum,
    version: prior.version + 1,
    updated_at: now,
    author_sub: subject,
  };
}
function pointer(source: DriveVersionSource): DriveBodyPointer {
  return {
    ...source.binary,
    origin: source.binary.origin ?? {
      kind: source.item.source_kind,
      id: source.item.source_id,
      epoch: source.item.epoch,
    },
  };
}
function checkSource(
  source: DriveVersionSource,
  request: DriveRestoreRequest,
  prior: DriveSummary,
) {
  const item = source.item;
  if (
    item.version !== request.version ||
    item.version >= prior.version ||
    !(
      (item.source_kind === 'file' && item.source_id === request.id) ||
      (item.source_kind === 'version' && item.parent_id === request.id)
    )
  )
    invalid();
}
async function capture(
  host: DriveRestoreHost,
  request: DriveRestoreRequest,
  input: string,
  signal: AbortSignal,
): Promise<RestoreIntent> {
  const previous = await host.capture(request, signal);
  try {
    checkSource(previous.source, request, previous.item);
    const now = new Date().toISOString(),
      raw = patchDriveJSON(
        previous.raw,
        patch(previous.source, previous.item, now, host.repository.scope.subject),
      );
    try {
      const item = {
        ...previous.item,
        ...patch(previous.source, previous.item, now, host.repository.scope.subject),
        seed: false,
        raw_sha256: await digest(raw),
      };
      await verifyProjection(item, 'live', raw);
      return {
        v: 1,
        operation: 'restore_version',
        scope: { ...host.repository.scope },
        input,
        request: { ...request },
        target: host.repository.publicationTarget(),
        revision: crypto.randomUUID(),
        base: previous.base.reference,
        item,
        raw: encodeStorageJSON(raw),
        binary: pointer(previous.source),
        restored: {
          object_id: previous.restored.object_id,
          reference: previous.restored.reference,
        },
      };
    } finally {
      raw.fill(0);
    }
  } finally {
    previous.raw.fill(0);
  }
}
async function decode(
  bytes: Uint8Array<ArrayBuffer>,
  host: DriveRestoreHost,
  request: DriveRestoreRequest,
  input: string,
  signal: AbortSignal,
): Promise<RestoreIntent> {
  if (bytes.length > 256 << 10) invalid();
  const value = fields(
      JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown,
    ),
    scope = storageScope(value.scope),
    item = parseSummary(value.item);
  if (
    value.v !== 1 ||
    value.operation !== 'restore_version' ||
    value.input !== input ||
    !equal(value.request, request) ||
    !equal(scope, host.repository.scope) ||
    !storageIdentifier(value.revision) ||
    item.source_kind !== 'file' ||
    item.source_id !== request.id ||
    item.seed ||
    value.binary_work != null
  )
    invalid();
  const raw = storageJSONBytes(value.raw, maxLegacySourceBytes),
    base = storageReference(value.base);
  try {
    const previous = await host.read(
      await host.repository.object(base, await driveObject('file', request.id), signal),
      signal,
    );
    try {
      if (
        previous.item.deleted_at !== null ||
        previous.item.epoch !== item.epoch ||
        previous.item.baseline_sha256 !== item.baseline_sha256
      )
        invalid();
      const savedSource = fields(value.restored);
      if (!storageIdentifier(savedSource.object_id)) invalid();
      const restored = {
          object_id: savedSource.object_id,
          reference: storageReference(savedSource.reference),
        },
        source = await host.source(
          await host.repository.object(restored.reference, restored.object_id, signal),
          signal,
        );
      checkSource(source, request, previous.item);
      const binary = await bodyPointer(value.binary, item, 'binary');
      if (!equal(binary, pointer(source))) invalid();
      const expected = patchDriveJSON(
        previous.raw,
        patch(source, previous.item, item.updated_at, scope.subject),
      );
      try {
        if (encodeStorageJSON(expected) !== value.raw) invalid();
      } finally {
        expected.fill(0);
      }
      await verifyProjection(item, 'live', raw);
      const intent: RestoreIntent = {
        v: 1,
        operation: 'restore_version',
        scope,
        input,
        request: { ...request },
        target: host.repository.publicationTarget(value.target as StoragePublicationTarget),
        revision: value.revision,
        base,
        item,
        raw: value.raw,
        binary,
        restored,
      };
      decodeDriveEntryCheckpoints(value, intent, raw.length);
      await host.validate(item, base, intent.revision, signal);
      return intent;
    } finally {
      previous.raw.fill(0);
    }
  } finally {
    raw.fill(0);
  }
}
/** All metadata is frozen before effects; a retry never reselects the archive. */
export async function restoreDriveVersion(
  host: DriveRestoreHost,
  request: DriveRestoreRequest,
  signal: AbortSignal,
): Promise<void> {
  const { journal } = host.repository.writeContext(),
    input = await digest(
      encoder.encode(
        JSON.stringify({ scope: host.repository.scope, operation: 'restore_version', request }),
      ),
    ),
    key = 'drive_restore_version_' + input;
  let stored = await journal.read(key);
  if (stored === null) {
    const intent = await capture(host, request, input, signal);
    await host.validate(intent.item, intent.base, intent.revision, signal);
    await saveDriveWriteIntent(journal, key, intent, signal);
    stored = encoder.encode(JSON.stringify(intent));
  }
  let intent: RestoreIntent;
  try {
    intent = await decode(stored, host, request, input, signal);
  } finally {
    stored.fill(0);
  }
  await publishDriveEntry(host, journal, key, intent, signal);
  await journal.delete(key);
}
