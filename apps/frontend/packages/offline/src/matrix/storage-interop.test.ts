/* SPDX-License-Identifier: Apache-2.0 */
import { beforeAll, expect, test } from 'bun:test';
import { writeFile } from 'node:fs/promises';
import {
  CollectStrategy,
  DecryptionSettings,
  DeviceId,
  EncryptionAlgorithm,
  EncryptionSettings,
  HistoryVisibility,
  OlmMachine,
  RoomId,
  TrustRequirement,
  UserId,
  initAsync,
} from '@matrix-org/matrix-sdk-crypto-wasm';
import go from '../../../../../../test/fixtures/storage-matrix-go.json';
import {
  createStorageRecord,
  decodeStorageRecord,
  encodeStorageJSON,
  storageCatalogHeads,
  storageJSONBytes,
  storageObjectKind,
  type StorageRecordEntry,
  type StorageObjectCommit,
} from './storage.js';
import type { MatrixStorageScope } from './types.js';
beforeAll(async () => {
  await initAsync();
});
const scope: MatrixStorageScope = { tenant: 'acme', subject: 'person', product: 'drive' };
const room = '!private:matrix.test';
const text = new TextEncoder();
const metadata = text.encode(
  '{"title":"private <filename>","amount":900719925474099312345,"fraction":1.00000000000000000001}',
);
function entries(): StorageRecordEntry[] {
  const result: StorageRecordEntry[] = [
    {
      reference: { room_id: room, event_id: '$content' },
      record: createStorageRecord(scope, 'object', 'content', 'content', metadata),
    },
  ];
  for (const revision of ['base', 'edit', 'deleted']) {
    const commit: StorageObjectCommit = {
      kind: storageObjectKind,
      state: revision === 'deleted' ? 'deleted' : 'live',
      metadata: encodeStorageJSON(metadata),
      ...(revision !== 'deleted'
        ? { content: { kind: 'record', revision_id: 'content', reference: result[0]!.reference } }
        : {}),
      ...(revision !== 'base' ? { parents: [{ room_id: room, event_id: '$base' }] } : {}),
    };
    result.push({
      reference: { room_id: room, event_id: '$' + revision },
      record: createStorageRecord(
        scope,
        'object',
        revision,
        'index',
        text.encode(JSON.stringify(commit)),
        (commit.parents ?? []).map((parent) => parent.event_id),
      ),
    });
  }
  return result;
}

test('Rust SDK produces private records consumed independently by native Go without number loss', async () => {
  const machine = await OlmMachine.initialize(
    new UserId('@person:matrix.test'),
    new DeviceId('ARTIFICIAL_STORAGE_JS'),
  );
  const settings = new EncryptionSettings();
  settings.algorithm = EncryptionAlgorithm.MegolmV1AesSha2;
  settings.historyVisibility = HistoryVisibility.Joined;
  settings.sharingStrategy = CollectStrategy.deviceBasedStrategy(true, true);
  try {
    const requests = await machine.shareRoomKey(
      new RoomId(room),
      [new UserId('@person:matrix.test')],
      settings,
    );
    for (const request of requests) request.free();
    const records = entries();
    const events = [];
    for (const entry of records) {
      const content = JSON.parse(
        await machine.encryptRoomEvent(
          new RoomId(room),
          'm.room.message',
          JSON.stringify({ msgtype: 'm.text', body: '', 'org.sytue.storage': entry.record }),
        ),
      );
      events.push({
        type: 'm.room.encrypted',
        room_id: room,
        event_id: entry.reference.event_id,
        sender: '@person:matrix.test',
        origin_server_ts: 1792152000000,
        content,
      });
    }
    const sessions = JSON.parse(await machine.exportRoomKeys(() => true));
    const vector = {
      artificial: true,
      generator: 'matrix-sdk-crypto-wasm 18.9.0; ARTIFICIAL PRIVATE STORAGE ONLY',
      scope,
      entries: records,
      sessions,
      events,
      heads: ['$deleted', '$edit'],
    };
    expect(sessions.length).toBeGreaterThan(0);
    expect(JSON.stringify(events)).not.toContain('private <filename>');
    const output = process.env.SYTUE_STORAGE_JS_VECTOR_OUT;
    if (output) await writeFile(output, JSON.stringify(vector, null, 2) + '\n', { mode: 0o600 });
  } finally {
    settings.free();
    machine.close();
  }
});

test('Rust decrypts independently generated Go records and preserves concurrent native catalog heads', async () => {
  expect(go.artificial).toBe(true);
  const machine = await OlmMachine.initialize(
    new UserId('@reader:matrix.test'),
    new DeviceId('ARTIFICIAL_STORAGE_READER'),
  );
  const settings = new DecryptionSettings(TrustRequirement.Untrusted);
  try {
    await machine.importExportedRoomKeys(JSON.stringify(go.sessions), () => undefined);
    const indexes: StorageRecordEntry[] = [];
    for (let i = 0; i < go.events.length; i++) {
      const raw = go.events[i]!;
      const result = await machine.decryptRoomEvent(
        JSON.stringify(raw),
        new RoomId(raw.room_id),
        settings,
      );
      try {
        const event = JSON.parse(result.event);
        const record = decodeStorageRecord(event.content['org.sytue.storage'], scope);
        expect(record).toEqual(decodeStorageRecord(go.entries[i]!.record, scope));
        if (record.role === 'content') expect(storageJSONBytes(record.payload)).toEqual(metadata);
        if (record.role === 'index') indexes.push({ reference: go.entries[i]!.reference, record });
        // Exported fixtures carry no live sender verification; never claim that they do.
        expect(result.shieldState(true)).not.toBeNull();
      } finally {
        result.free();
      }
    }
    expect(
      storageCatalogHeads(scope, indexes).map((version) => version.reference.event_id),
    ).toEqual(go.heads);
    const raw = structuredClone(go.events[0]!);
    raw.content.ciphertext = raw.content.ciphertext.slice(0, -8) + 'AAAAAAAA';
    await expect(
      machine.decryptRoomEvent(JSON.stringify(raw), new RoomId(raw.room_id), settings),
    ).rejects.toThrow();
  } finally {
    settings.free();
    machine.close();
  }
});
