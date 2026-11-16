/* SPDX-License-Identifier: Apache-2.0 */
/** New identities and their exact metadata are journaled before any remote effect. */
import { maxLegacySourceBytes } from '@sytue/config/matrix';
import {
  encodeStorageJSON,
  storageJSONBytes,
  storageReference,
  storageScope,
  storageIdentifier,
  type StoragePublicationTarget,
} from './storage.js';
import { fields, digest, invalid } from './storage-file-codec.js';
import {
  driveItemKind,
  parseSummary,
  verifyProjection,
  json,
  type DriveSummary,
} from './private-drive-codec.js';
import {
  publishDriveEntry,
  saveDriveWriteIntent,
  type DriveEntryHost,
  type DriveWriteIntent,
} from './private-drive-metadata.js';
interface FolderIntent extends DriveWriteIntent {
  v: 1;
  operation: 'create_folder';
  input: string;
  parent: string;
  name: string;
}
export interface DriveCreateHost extends DriveEntryHost {
  validate: (item: DriveSummary, revision: string, signal: AbortSignal) => Promise<void>;
}
const encoder = new TextEncoder();
function folderBytes(
  id: string,
  parent: string,
  name: string,
  owner: string,
  now: string,
): Uint8Array<ArrayBuffer> {
  return encoder.encode(
    JSON.stringify({
      id,
      parent_id: parent,
      name,
      is_root: false,
      is_group: false,
      owner_sub: owner,
      created_at: now,
      updated_at: now,
    }),
  );
}
async function decode(
  bytes: Uint8Array<ArrayBuffer>,
  host: DriveCreateHost,
  parent: string,
  name: string,
  input: string,
  signal: AbortSignal,
): Promise<FolderIntent> {
  if (bytes.length > 256 << 10) invalid();
  const value = fields(
      JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown,
    ),
    item = parseSummary(value.item),
    scope = storageScope(value.scope);
  if (
    value.v !== 1 ||
    value.operation !== 'create_folder' ||
    value.input !== input ||
    value.parent !== parent ||
    value.name !== name ||
    JSON.stringify(scope) !== JSON.stringify(host.repository.scope) ||
    !storageIdentifier(value.revision) ||
    item.source_kind !== 'folder' ||
    item.parent_id !== parent ||
    item.name !== name ||
    item.is_root ||
    item.deleted_at !== null ||
    item.epoch !== '' ||
    item.baseline_sha256 !== '' ||
    item.seed ||
    value.base != null ||
    value.binary != null ||
    value.metadata != null
  )
    invalid();
  const raw = storageJSONBytes(value.raw, maxLegacySourceBytes),
    expected = folderBytes(item.source_id, parent, name, scope.subject, item.updated_at);
  try {
    if (encodeStorageJSON(expected) !== value.raw) invalid();
    await verifyProjection(item, 'live', raw);
    const intent: FolderIntent = {
      v: 1,
      operation: 'create_folder',
      scope,
      input,
      parent,
      name,
      target: host.repository.publicationTarget(value.target as StoragePublicationTarget),
      revision: value.revision,
      item,
      raw: value.raw,
    };
    if (value.content != null) intent.content = storageReference(value.content);
    if (value.catalog != null) intent.catalog = storageReference(value.catalog);
    if (intent.catalog && !intent.content) invalid();
    await host.validate(item, intent.revision, signal);
    return intent;
  } finally {
    raw.fill(0);
    expected.fill(0);
  }
}
/** The caller holds the same device catalog queue used by metadata edits. */
export async function createDriveFolder(
  host: DriveCreateHost,
  parent: string,
  name: string,
  signal: AbortSignal,
): Promise<Record<string, unknown>> {
  const repository = host.repository,
    { journal: cache } = repository.writeContext(),
    input = await digest(
      encoder.encode(
        JSON.stringify({ scope: repository.scope, operation: 'create_folder', parent, name }),
      ),
    ),
    key = 'drive_create_folder_' + input;
  let stored = await cache.read(key);
  if (stored === null) {
    const id = crypto.randomUUID(),
      now = new Date().toISOString(),
      raw = folderBytes(id, parent, name, repository.scope.subject, now);
    try {
      const item = parseSummary({
          kind: driveItemKind,
          source_kind: 'folder',
          source_id: id,
          parent_id: parent,
          name,
          updated_at: now,
          raw_sha256: await digest(raw),
        }),
        intent: FolderIntent = {
          v: 1,
          operation: 'create_folder',
          scope: { ...repository.scope },
          input,
          parent,
          name,
          target: repository.publicationTarget(),
          revision: crypto.randomUUID(),
          item,
          raw: encodeStorageJSON(raw),
        };
      await host.validate(item, intent.revision, signal);
      await saveDriveWriteIntent(cache, key, intent, signal);
      stored = encoder.encode(JSON.stringify(intent));
    } finally {
      raw.fill(0);
    }
  }
  let intent: FolderIntent;
  try {
    intent = await decode(stored, host, parent, name, input, signal);
  } finally {
    stored.fill(0);
  }
  await publishDriveEntry(host, cache, key, intent, signal);
  await cache.delete(key);
  const raw = storageJSONBytes(intent.raw, maxLegacySourceBytes);
  try {
    return json(raw);
  } finally {
    raw.fill(0);
  }
}
