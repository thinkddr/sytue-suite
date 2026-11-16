/* SPDX-License-Identifier: Apache-2.0 */
/** Durable metadata edits. Original source bytes and binary media never leave the device in plaintext. */
import { maxLegacySourceBytes, type MatrixStorageScope } from '@sytue/config/matrix';
import { storageBlobSource, type StorageFileSource } from './storage-file-upload.js';
import type { StorageRepository, StorageWorkflowJournal } from './storage-repository.js';
import {
  storageReference,
  storageScope,
  storageIdentifier,
  createStorageRecord,
  storageJSONBytes,
  encodeStorageJSON,
  storageObjectKind,
  type StorageObjectVersion,
  type StoragePublicationTarget,
} from './storage.js';
import type { MatrixReference } from './types.js';
import { fields, digest, invalid, hash, type StorageFileRequest } from './storage-file-codec.js';
import {
  driveObject,
  driveBodyKind,
  parseSummary,
  patchDriveJSON,
  verifyProjection,
  bodyPointer,
  type DriveSummary,
  type DriveBodyPointer,
  type DriveEntry,
} from './private-drive-codec.js';
export interface DriveMetadataRequest {
  operation: 'rename' | 'move' | 'trash' | 'restore';
  kind: 'file' | 'folder';
  id: string;
  value: string;
}
export interface DriveMetadataRead {
  item: DriveSummary;
  entry: DriveEntry;
  raw: Uint8Array<ArrayBuffer>;
}
export interface DriveMetadataCapture {
  base: StorageObjectVersion;
  item: DriveSummary;
  raw: Uint8Array<ArrayBuffer>;
  binary?: DriveBodyPointer;
}
export interface DriveEntryHost {
  repository: StorageRepository;
  read: (version: StorageObjectVersion, signal: AbortSignal) => Promise<DriveMetadataRead>;
}
export interface DriveMetadataHost extends DriveEntryHost {
  capture: (
    request: DriveMetadataRequest,
    signal: AbortSignal,
  ) => Promise<DriveMetadataCapture | null>;
}
export interface DriveWriteIntent {
  scope: MatrixStorageScope;
  target: StoragePublicationTarget;
  revision: string;
  base?: MatrixReference;
  item: DriveSummary;
  raw: string;
  binary?: DriveBodyPointer;
  content?: MatrixReference;
  catalog?: MatrixReference;
  metadata?: DriveBodyWork;
  binary_work?: DriveBodyWork;
}
export interface DriveBodyWork {
  revision: string;
  parents: MatrixReference[];
  file?: MatrixReference;
  catalog?: MatrixReference;
  acknowledged?: boolean;
  sha256?: string;
}
interface Intent extends DriveWriteIntent {
  v: 1;
  input: string;
  request: DriveMetadataRequest;
  base: MatrixReference;
}
const encoder = new TextEncoder();
const equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
export function metadataPatch(
  request: DriveMetadataRequest,
  item: DriveSummary,
): Record<string, unknown> {
  const patch: Record<string, unknown> = { updated_at: item.updated_at };
  switch (request.operation) {
    case 'rename':
      patch.name = request.value;
      break;
    case 'move':
      patch[request.kind === 'file' ? 'folder_id' : 'parent_id'] = request.value;
      break;
    case 'trash':
      if (!item.deleted_at) invalid();
      patch.deleted_at = item.deleted_at;
      break;
    case 'restore':
      if (item.deleted_at !== null) invalid();
      patch.deleted_at = null;
      break;
  }
  return patch;
}
export async function saveDriveWriteIntent(
  cache: StorageWorkflowJournal,
  key: string,
  intent: DriveWriteIntent,
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  const bytes = encoder.encode(JSON.stringify(intent));
  try {
    if (bytes.length > 256 << 10) invalid();
    await cache.write(key, bytes);
    signal.throwIfAborted();
  } finally {
    bytes.fill(0);
  }
}
async function decode(
  bytes: Uint8Array<ArrayBuffer>,
  host: DriveMetadataHost,
  request: DriveMetadataRequest,
  input: string,
  signal: AbortSignal,
): Promise<Intent> {
  if (bytes.length > 256 << 10) invalid();
  const value = fields(
    JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown,
  );
  const item = parseSummary(value.item),
    scope = storageScope(value.scope),
    target = host.repository.publicationTarget(value.target as StoragePublicationTarget);
  if (
    value.v !== 1 ||
    value.input !== input ||
    !equal(value.request, request) ||
    !equal(scope, host.repository.scope) ||
    !storageIdentifier(value.revision) ||
    item.source_kind !== request.kind ||
    item.source_id !== request.id ||
    item.seed
  )
    invalid();
  const raw = storageJSONBytes(value.raw, maxLegacySourceBytes),
    base = storageReference(value.base),
    object = await driveObject(request.kind, request.id);
  const previous = await host.read(await host.repository.object(base, object, signal), signal);
  try {
    if (
      previous.item.is_root ||
      previous.item.epoch !== item.epoch ||
      previous.item.baseline_sha256 !== item.baseline_sha256
    )
      invalid();
    const binary =
      value.binary == null ? undefined : await bodyPointer(value.binary, item, 'binary');
    if (!equal(binary, previous.entry.binary)) invalid();
    const expected = patchDriveJSON(previous.raw, metadataPatch(request, item));
    try {
      if (encodeStorageJSON(expected) !== value.raw) invalid();
    } finally {
      expected.fill(0);
    }
    await verifyProjection(item, item.deleted_at === null ? 'live' : 'trash', raw);
    const intent: Intent = {
      v: 1,
      scope,
      input,
      request: { ...request },
      target,
      revision: value.revision,
      base,
      item,
      raw: value.raw,
      ...(binary ? { binary } : {}),
    };
    decodeDriveEntryCheckpoints(value, intent, raw.length);
    return intent;
  } finally {
    raw.fill(0);
    previous.raw.fill(0);
  }
}
/** Shared validation for durable entry publication checkpoints. */
export function decodeDriveEntryCheckpoints(
  value: Record<string, unknown>,
  intent: DriveWriteIntent,
  rawLength: number,
): void {
  if (value.content != null) intent.content = storageReference(value.content);
  if (value.catalog != null) intent.catalog = storageReference(value.catalog);
  if (value.metadata != null) intent.metadata = decodeDriveBodyWork(value.metadata);
  if (intent.catalog && !intent.content) invalid();
  if (rawLength <= 16 << 10 && intent.metadata) invalid();
  if (rawLength > 16 << 10 && intent.content && !intent.metadata?.catalog) invalid();
  if (intent.metadata?.catalog && !intent.metadata.file) invalid();
  if (intent.metadata?.acknowledged && !intent.metadata.catalog) invalid();
}
export function decodeDriveBodyWork(input: unknown): DriveBodyWork {
  const value = fields(input);
  if (
    !storageIdentifier(value.revision) ||
    !Array.isArray(value.parents) ||
    value.parents.length > 16
  )
    invalid();
  const parents = value.parents.map(storageReference);
  if (new Set(parents.map((r) => JSON.stringify(r))).size !== parents.length) invalid();
  const work: DriveBodyWork = {
    revision: value.revision,
    parents,
    ...(value.file != null ? { file: storageReference(value.file) } : {}),
    ...(value.catalog != null ? { catalog: storageReference(value.catalog) } : {}),
    ...(value.acknowledged === true ? { acknowledged: true } : {}),
    ...(value.sha256 !== undefined ? { sha256: value.sha256 as string } : {}),
  };
  if (work.sha256 !== undefined && !hash(work.sha256)) invalid();
  if ((work.catalog && !work.file) || (work.acknowledged && !work.catalog)) invalid();
  return work;
}
export async function publishDriveBody(
  host: DriveEntryHost,
  cache: StorageWorkflowJournal,
  key: string,
  intent: DriveWriteIntent,
  purpose: 'metadata' | 'binary',
  size: number,
  source: StorageFileSource | undefined,
  signal: AbortSignal,
): Promise<DriveBodyPointer> {
  const repository = host.repository,
    object = await driveObject(intent.item.source_kind + '_' + purpose, intent.item.source_id),
    field = purpose === 'metadata' ? 'metadata' : 'binary_work',
    sha256 =
      purpose === 'metadata'
        ? intent.item.raw_sha256
        : intent.item.checksum || intent[field]?.sha256 || source?.sha256;
  if (
    !hash(sha256) ||
    (purpose === 'binary' && size !== intent.item.size_bytes) ||
    (source && source.size !== size)
  )
    invalid();
  if (!intent[field]) {
    const parents = (await repository.catalog(signal))
      .filter((v) => v.object_id === object)
      .map((v) => v.reference);
    if (parents.length > 16) invalid();
    intent[field] = { revision: crypto.randomUUID(), parents, sha256 };
    await saveDriveWriteIntent(cache, key, intent, signal);
  }
  const work = intent[field] ?? invalid(),
    job = (purpose === 'metadata' ? 'drive_meta_body_' : 'drive_binary_body_') + intent.revision;
  const request: StorageFileRequest = {
    object_id: object,
    revision_id: work.revision,
    size_bytes: size,
    metadata: encodeStorageJSON(
      encoder.encode(
        JSON.stringify({
          kind: driveBodyKind,
          source_kind: intent.item.source_kind,
          source_id: intent.item.source_id,
          purpose,
          epoch: intent.item.epoch,
        }),
      ),
    ),
  };
  if (work.sha256 !== undefined && work.sha256 !== sha256) invalid();
  const uploads = repository.uploads(intent.target);
  if (!work.file) {
    if (!source) invalid();
    work.file = await uploads.uploadSource(job, request, { ...source, sha256 }, signal);
    await saveDriveWriteIntent(cache, key, intent, signal);
  }
  if (!work.catalog) {
    const body = {
      kind: driveBodyKind,
      source_kind: intent.item.source_kind,
      source_id: intent.item.source_id,
      epoch: intent.item.epoch,
      purpose,
      sha256,
      size_bytes: size,
    };
    const version = await repository.putObject(
      work.revision + '_index',
      object,
      work.revision + '_index',
      {
        kind: storageObjectKind,
        state: 'live',
        metadata: encodeStorageJSON(encoder.encode(JSON.stringify(body))),
        content: { kind: 'file', reference: work.file, revision_id: work.revision },
        parents: work.parents,
      },
      signal,
      intent.target,
    );
    work.catalog = version.reference;
    await saveDriveWriteIntent(cache, key, intent, signal);
  }
  const confirmed = await repository.object(work.catalog, object, signal),
    description = {
      kind: driveBodyKind,
      source_kind: intent.item.source_kind,
      source_id: intent.item.source_id,
      epoch: intent.item.epoch,
      purpose,
      sha256,
      size_bytes: size,
    };
  if (
    confirmed.commit.metadata !== encodeStorageJSON(encoder.encode(JSON.stringify(description))) ||
    !equal(confirmed.commit.parents ?? [], work.parents)
  )
    invalid();
  await repository.verifyFileCatalog(work.catalog, work.file, request, sha256, signal);
  if (!work.acknowledged) {
    await uploads.acknowledge(job, request, work.catalog, signal);
    work.acknowledged = true;
    await saveDriveWriteIntent(cache, key, intent, signal);
  }
  return { object_id: object, reference: work.catalog };
}
/** Caller holds the SDK catalog queue and refreshes its authenticated projection first. */
export async function mutateDriveMetadata(
  host: DriveMetadataHost,
  request: DriveMetadataRequest,
  signal: AbortSignal,
): Promise<void> {
  const repository = host.repository,
    { journal: cache } = repository.writeContext(),
    input = await digest(encoder.encode(JSON.stringify({ scope: repository.scope, request }))),
    key = 'drive_metadata_' + input;
  const saved = await cache.read(key);
  let intent: Intent;
  if (saved !== null) {
    try {
      intent = await decode(saved, host, request, input, signal);
    } finally {
      saved.fill(0);
    }
  } else {
    const capture = await host.capture(request, signal);
    if (!capture) return;
    try {
      intent = {
        v: 1,
        scope: { ...repository.scope },
        input,
        request: { ...request },
        target: repository.publicationTarget(),
        revision: crypto.randomUUID(),
        base: capture.base.reference,
        item: capture.item,
        raw: encodeStorageJSON(capture.raw),
        ...(capture.binary ? { binary: capture.binary } : {}),
      };
      await saveDriveWriteIntent(cache, key, intent, signal);
    } finally {
      capture.raw.fill(0);
    }
    // Apply the same validation to fresh and resumed local data.
    const bytes = encoder.encode(JSON.stringify(intent));
    try {
      intent = await decode(bytes, host, request, input, signal);
    } finally {
      bytes.fill(0);
    }
  }
  await publishDriveEntry(host, cache, key, intent, signal);
  await cache.delete(key);
}
/** One durable publication path for edits, new folders and subsequent Drive writes. */
export async function publishDriveEntry(
  host: DriveEntryHost,
  cache: StorageWorkflowJournal,
  key: string,
  intent: DriveWriteIntent,
  signal: AbortSignal,
): Promise<void> {
  const repository = host.repository;
  const object = await driveObject(intent.item.source_kind, intent.item.source_id),
    raw = storageJSONBytes(intent.raw, maxLegacySourceBytes);
  try {
    let metadata: DriveBodyPointer | undefined;
    if (raw.length > 16 << 10)
      metadata = await publishDriveBody(
        host,
        cache,
        key,
        intent,
        'metadata',
        raw.length,
        storageBlobSource(new Blob([raw])),
        signal,
      );
    if (!intent.content) {
      const payload = encoder.encode(
        JSON.stringify({
          kind: 'org.sytue.drive.entry.v1',
          source_kind: intent.item.source_kind,
          source_id: intent.item.source_id,
          ...(metadata ? { metadata } : { raw: intent.raw }),
          ...(intent.binary ? { binary: intent.binary } : {}),
        }),
      );
      try {
        intent.content = await repository.putRecord(
          intent.revision + '_content',
          createStorageRecord(repository.scope, object, intent.revision, 'content', payload),
          signal,
          intent.target,
        );
      } finally {
        payload.fill(0);
      }
      await saveDriveWriteIntent(cache, key, intent, signal);
    }
    if (!intent.catalog) {
      const result = await repository.putObject(
        intent.revision + '_index',
        object,
        intent.revision + '_index',
        {
          kind: storageObjectKind,
          state: intent.item.deleted_at === null ? 'live' : 'trash',
          metadata: encodeStorageJSON(encoder.encode(JSON.stringify(intent.item))),
          content: { kind: 'record', reference: intent.content, revision_id: intent.revision },
          parents: intent.base ? [intent.base] : [],
        },
        signal,
        intent.target,
      );
      intent.catalog = result.reference;
      await saveDriveWriteIntent(cache, key, intent, signal);
    }
    const confirmed = await host.read(
      await repository.object(intent.catalog, object, signal),
      signal,
    );
    try {
      if (
        encodeStorageJSON(confirmed.raw) !== intent.raw ||
        !equal(confirmed.entry.binary, intent.binary)
      )
        invalid();
    } finally {
      confirmed.raw.fill(0);
    }
    signal.throwIfAborted();
    repository.assertOpen(signal);
  } finally {
    raw.fill(0);
  }
}
