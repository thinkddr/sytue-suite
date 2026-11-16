/* SPDX-License-Identifier: Apache-2.0 */
/** Device-owned Drive uploads: captured source, pinned revisions and authenticated completion. */
import { isStorageText, legacySourceCursor, maxLegacySourceBytes } from '@sytue/config/matrix';
import {
  storageJSONBytes,
  encodeStorageJSON,
  storageScope,
  storageReference,
  storageIdentifier,
  type StorageObjectVersion,
  type StoragePublicationTarget,
} from './storage.js';
import { storageStagedFile, type StorageStagedFile } from './storage-staged-files.js';
import { fields, digest, invalid, maxStorageFileBytes } from './storage-file-codec.js';
import {
  parseSummary,
  driveItemKind,
  driveName,
  driveObject,
  verifyProjection,
  patchDriveJSON,
  bodyPointer,
  fileView,
  json,
  type DriveSummary,
} from './private-drive-codec.js';
import {
  saveDriveWriteIntent,
  publishDriveEntry,
  publishDriveBody,
  decodeDriveBodyWork,
  type DriveWriteIntent,
  type DriveEntryHost,
} from './private-drive-metadata.js';
export interface DriveUploadRequest {
  folder: string;
  name: string;
  content_type: string;
  size_bytes: number;
}
export interface DriveUploadHost extends DriveEntryHost {
  destination: (
    request: DriveUploadRequest,
    signal: AbortSignal,
  ) => Promise<StorageObjectVersion | undefined>;
  validate: (
    item: DriveSummary,
    base: DriveWriteIntent['base'],
    revision: string,
    signal: AbortSignal,
  ) => Promise<void>;
  refresh: (signal: AbortSignal) => Promise<void>;
}
interface UploadIntent extends DriveWriteIntent {
  v: 1;
  operation: 'upload';
  input: string;
  request: DriveUploadRequest;
  staged: StorageStagedFile;
}
const encoder = new TextEncoder(),
  equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
export function driveUploadRequest(input: unknown): DriveUploadRequest {
  const row = fields(input);
  legacySourceCursor({ kind: 'folder', id: row.folder });
  if (
    typeof row.name !== 'string' ||
    typeof row.content_type !== 'string' ||
    !Number.isSafeInteger(row.size_bytes) ||
    (row.size_bytes as number) < 0 ||
    (row.size_bytes as number) > maxStorageFileBytes
  )
    invalid();
  const type = row.content_type || 'application/octet-stream';
  if (!isStorageText(type, 1024) || /[\r\n\0]/.test(type)) invalid();
  return {
    folder: row.folder as string,
    name: driveName(row.name),
    content_type: type,
    size_bytes: row.size_bytes as number,
  };
}
async function identity(host: DriveEntryHost, request: DriveUploadRequest) {
  const scope = host.repository.scope,
    prefix = 'drive_upload_' + (await digest(encoder.encode(JSON.stringify(scope)))) + '_',
    input = await digest(encoder.encode(JSON.stringify({ scope, request })));
  return { prefix, input, key: prefix + input };
}
function patch(
  request: DriveUploadRequest,
  sum: string,
  version: number,
  now: string,
  author: string,
) {
  return {
    size_bytes: request.size_bytes,
    content_type: request.content_type,
    checksum: sum,
    version,
    updated_at: now,
    author_sub: author,
  };
}
function fresh(request: DriveUploadRequest, sum: string, id: string, now: string, owner: string) {
  return encoder.encode(
    JSON.stringify({
      id,
      folder_id: request.folder,
      name: request.name,
      size_bytes: request.size_bytes,
      content_type: request.content_type,
      checksum: sum,
      version: 1,
      owner_sub: owner,
      created_at: now,
      updated_at: now,
    }),
  );
}
async function capture(
  host: DriveUploadHost,
  request: DriveUploadRequest,
  staged: StorageStagedFile,
  input: string,
  signal: AbortSignal,
): Promise<UploadIntent> {
  const repository = host.repository,
    base = await host.destination(request, signal),
    now = new Date().toISOString();
  let raw: Uint8Array<ArrayBuffer> | undefined;
  try {
    let item: DriveSummary;
    if (base) {
      const old = await host.read(base, signal);
      try {
        if (old.item.version < 1 || old.item.version >= 0x7fffffff) invalid();
        raw = patchDriveJSON(
          old.raw,
          patch(request, staged.sha256, old.item.version + 1, now, repository.scope.subject),
        );
        item = {
          ...old.item,
          seed: false,
          size_bytes: request.size_bytes,
          content_type: request.content_type,
          checksum: staged.sha256,
          version: old.item.version + 1,
          updated_at: now,
          raw_sha256: await digest(raw),
        };
      } finally {
        old.raw.fill(0);
      }
    } else {
      const id = crypto.randomUUID();
      raw = fresh(request, staged.sha256, id, now, repository.scope.subject);
      item = parseSummary({
        kind: driveItemKind,
        source_kind: 'file',
        source_id: id,
        parent_id: request.folder,
        name: request.name,
        size_bytes: request.size_bytes,
        content_type: request.content_type,
        checksum: staged.sha256,
        version: 1,
        updated_at: now,
        raw_sha256: await digest(raw),
      });
    }
    return {
      v: 1,
      operation: 'upload',
      scope: { ...repository.scope },
      input,
      request: { ...request },
      staged,
      target: repository.publicationTarget(),
      revision: crypto.randomUUID(),
      item,
      raw: encodeStorageJSON(raw),
      ...(base ? { base: base.reference } : {}),
    };
  } finally {
    raw?.fill(0);
  }
}
async function decode(
  bytes: Uint8Array<ArrayBuffer>,
  host: DriveUploadHost,
  request: DriveUploadRequest,
  input: string,
  signal: AbortSignal,
): Promise<UploadIntent> {
  if (bytes.length > 256 << 10) invalid();
  const value = fields(
      JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown,
    ),
    scope = storageScope(value.scope),
    item = parseSummary(value.item),
    staged = storageStagedFile(value.staged, host.repository.scope);
  if (
    value.v !== 1 ||
    value.operation !== 'upload' ||
    value.input !== input ||
    !equal(value.request, request) ||
    !equal(scope, host.repository.scope) ||
    !storageIdentifier(value.revision) ||
    item.source_kind !== 'file' ||
    item.parent_id !== request.folder ||
    item.name !== request.name ||
    item.content_type !== request.content_type ||
    item.size_bytes !== request.size_bytes ||
    item.checksum !== staged.sha256 ||
    item.size_bytes !== staged.size_bytes ||
    item.seed ||
    item.is_root ||
    item.deleted_at !== null
  )
    invalid();
  const intent: UploadIntent = {
    v: 1,
    operation: 'upload',
    scope,
    input,
    request: { ...request },
    staged,
    target: host.repository.publicationTarget(value.target as StoragePublicationTarget),
    revision: value.revision,
    item,
    raw: value.raw as string,
    ...(value.base != null ? { base: storageReference(value.base) } : {}),
  };
  const raw = storageJSONBytes(value.raw, maxLegacySourceBytes);
  let expected: Uint8Array<ArrayBuffer> | undefined;
  try {
    if (intent.base) {
      const old = await host.read(
        await host.repository.object(
          intent.base,
          await driveObject('file', item.source_id),
          signal,
        ),
        signal,
      );
      try {
        if (
          old.item.version < 1 ||
          old.item.version >= 0x7fffffff ||
          item.version !== old.item.version + 1 ||
          item.epoch !== old.item.epoch ||
          item.baseline_sha256 !== old.item.baseline_sha256 ||
          old.item.parent_id !== request.folder ||
          old.item.name !== request.name ||
          old.item.deleted_at !== null
        )
          invalid();
        expected = patchDriveJSON(
          old.raw,
          patch(request, staged.sha256, item.version, item.updated_at, scope.subject),
        );
      } finally {
        old.raw.fill(0);
      }
    } else {
      if (item.epoch !== '' || item.baseline_sha256 !== '' || item.version !== 1) invalid();
      expected = fresh(request, staged.sha256, item.source_id, item.updated_at, scope.subject);
    }
    if (encodeStorageJSON(expected) !== value.raw) invalid();
    await verifyProjection(item, 'live', raw);
    if (value.metadata != null) intent.metadata = decodeDriveBodyWork(value.metadata);
    if (value.binary_work != null) intent.binary_work = decodeDriveBodyWork(value.binary_work);
    if (value.binary != null) {
      intent.binary = await bodyPointer(value.binary, item, 'binary');
      if (
        intent.binary.origin ||
        !intent.binary_work?.catalog ||
        !equal(intent.binary.reference, intent.binary_work.catalog)
      )
        invalid();
    }
    if (value.content != null) intent.content = storageReference(value.content);
    if (value.catalog != null) intent.catalog = storageReference(value.catalog);
    if (
      (intent.content && !intent.binary) ||
      (intent.catalog && !intent.content) ||
      (raw.length <= 16 << 10 && intent.metadata) ||
      (raw.length > 16 << 10 && intent.content && !intent.metadata?.catalog)
    )
      invalid();
    await host.validate(item, intent.base, intent.revision, signal);
    return intent;
  } finally {
    raw.fill(0);
    expected?.fill(0);
  }
}
async function finish(
  host: DriveUploadHost,
  key: string,
  intent: UploadIntent,
  signal: AbortSignal,
): Promise<Record<string, unknown>> {
  const repository = host.repository,
    { journal } = repository.writeContext(),
    stager = repository.stagedFiles();
  const source = intent.binary_work?.file ? undefined : await stager.open(intent.staged, signal);
  const binary = await publishDriveBody(
    host,
    journal,
    key,
    intent,
    'binary',
    intent.item.size_bytes,
    source,
    signal,
  );
  if (intent.binary && !equal(intent.binary, binary)) invalid();
  intent.binary = binary;
  await saveDriveWriteIntent(journal, key, intent, signal);
  await publishDriveEntry(host, journal, key, intent, signal);
  await stager.discard(intent.staged, signal);
  await journal.delete(key);
  const raw = storageJSONBytes(intent.raw, maxLegacySourceBytes);
  try {
    return fileView(json(raw), intent.item.source_id);
  } finally {
    raw.fill(0);
  }
}
export async function uploadDriveFile(
  host: DriveUploadHost,
  request: DriveUploadRequest,
  source: Blob,
  signal: AbortSignal,
): Promise<Record<string, unknown>> {
  const repository = host.repository,
    { journal } = repository.writeContext(),
    { input, key } = await identity(host, request),
    stager = repository.stagedFiles();
  // Capture independently on explicit retry: retained keys must never encrypt a changed original.
  const staged = await stager.capture(source, signal);
  let retained = false;
  try {
    let bytes = await journal.read(key);
    if (bytes === null) {
      const intent = await capture(host, request, staged, input, signal);
      retained = true;
      await saveDriveWriteIntent(journal, key, intent, signal);
      bytes = encoder.encode(JSON.stringify(intent));
    }
    let intent: UploadIntent;
    try {
      intent = await decode(bytes, host, request, input, signal);
    } finally {
      bytes.fill(0);
    }
    if (intent.staged.sha256 !== staged.sha256 || intent.staged.size_bytes !== staged.size_bytes)
      invalid();
    return await finish(host, key, intent, signal);
  } finally {
    if (!retained) await stager.discard(staged, signal);
  }
}
export async function resumeDriveUploads(
  host: DriveUploadHost,
  signal: AbortSignal,
): Promise<{ completed: Record<string, unknown>[]; failed: { id: string; error: unknown }[] }> {
  const { journal } = host.repository.writeContext(),
    prefix =
      'drive_upload_' + (await digest(encoder.encode(JSON.stringify(host.repository.scope)))) + '_',
    ids = await journal.listIDs(10000, prefix),
    completed: Record<string, unknown>[] = [],
    failed: { id: string; error: unknown }[] = [];
  for (const id of ids) {
    signal.throwIfAborted();
    const bytes = await journal.read(id);
    if (bytes === null) continue;
    try {
      const value = fields(
          JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown,
        ),
        request = driveUploadRequest(value.request),
        { key, input } = await identity(host, request);
      if (id !== key) invalid();
      const intent = await decode(bytes, host, request, input, signal);
      completed.push(await finish(host, id, intent, signal));
      await host.refresh(signal);
    } catch (error) {
      signal.throwIfAborted();
      failed.push({ id, error });
    } finally {
      bytes.fill(0);
    }
  }
  return { completed, failed };
}
