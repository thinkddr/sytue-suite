/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { encryptedStorageMemory } from '../../../../packages/offline/src/encrypted-storage-fixture.js';
import {
  LocalComposeDraftStore,
  type LegacyLocalDraftPort,
  type LocalComposeDraft,
} from './local-draft.js';

const draft: LocalComposeDraft = {
  from: 'sender@example.test',
  to: 'recipient@example.test',
  cc: '',
  bcc: '',
  subject: 'private draft subject',
  bodyHtml: '<p>private draft body</p>',
  attachments: [{ name: 'private.txt', url: 'https://example.test/private.txt' }],
  files: [{ name: 'attachment.txt', type: 'text/plain', contentBase64: 'c2VjcmV0', size: 6 }],
};

function legacyPort(initial: unknown | null = null) {
  let value = initial;
  const operations: string[] = [];
  const port: LegacyLocalDraftPort = {
    read: async () => {
      operations.push('read');
      return value;
    },
    delete: async () => {
      operations.push('delete');
      value = null;
    },
  };
  return { port, operations, value: () => value };
}

test('Mail composer draft is encrypted at rest and decrypts across store instances', async () => {
  const storage = encryptedStorageMemory();
  const legacy = legacyPort();
  const first = new LocalComposeDraftStore(
    'mail-owner',
    storage.create('mail-owner', 'mail.compose'),
    legacy.port,
  );
  await first.write(draft);

  const row = [...storage.records.values()].find(
    (record) => record.namespace === 'local-crypto-value-v1',
  );
  expect(row).toBeDefined();
  expect(row!.value).not.toEqual(JSON.stringify(draft));
  const sealed = row!.value as { ciphertext: Uint8Array };
  expect(new TextDecoder().decode(sealed.ciphertext)).not.toContain('private draft subject');

  const second = new LocalComposeDraftStore(
    'mail-owner',
    storage.create('mail-owner', 'mail.compose'),
    legacy.port,
  );
  expect(await second.read()).toEqual(draft);
});

test('legacy plaintext draft is deleted only after an encrypted round trip verifies', async () => {
  const storage = encryptedStorageMemory();
  const legacy = legacyPort(draft);
  const events: string[] = [];
  const port: LegacyLocalDraftPort = {
    read: async (...args) => {
      events.push('read');
      return legacy.port.read(...args);
    },
    delete: async (...args) => {
      events.push('delete');
      return legacy.port.delete(...args);
    },
  };
  const store = new LocalComposeDraftStore(
    'mail-owner',
    storage.create('mail-owner', 'mail.compose'),
    port,
  );

  expect(await store.read()).toEqual(draft);
  expect(events).toEqual(['read', 'delete']);
  expect(legacy.value()).toBeNull();
});

test('corrupt encrypted draft fails closed without falling back to legacy plaintext', async () => {
  const storage = encryptedStorageMemory();
  const values = storage.create('mail-owner', 'mail.compose');
  await values.write('active', new TextEncoder().encode(JSON.stringify(draft)));
  const value = [...storage.records.values()].find(
    (record) => record.namespace === 'local-crypto-value-v1',
  )!.value as { ciphertext: Uint8Array };
  value.ciphertext[0]! ^= 1;
  const legacy = legacyPort(draft);
  const store = new LocalComposeDraftStore('mail-owner', values, legacy.port);

  await expect(store.read()).rejects.toThrow('encrypted_offline_unavailable');
  expect(legacy.operations).toEqual([]);
  expect(legacy.value()).toEqual(draft);
});

test('invalid legacy draft is not discarded during migration', async () => {
  const storage = encryptedStorageMemory();
  const legacy = legacyPort({ subject: 'not a complete draft' });
  const store = new LocalComposeDraftStore(
    'mail-owner',
    storage.create('mail-owner', 'mail.compose'),
    legacy.port,
  );

  await expect(store.read()).rejects.toThrow('encrypted_offline_unavailable');
  expect(legacy.operations).toEqual(['read']);
  expect(legacy.value()).toEqual({ subject: 'not a complete draft' });
  expect(
    [...storage.records.values()].some((record) => record.namespace === 'local-crypto-value-v1'),
  ).toBe(false);
});
