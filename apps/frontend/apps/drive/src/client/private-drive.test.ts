/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { DriveClient } from '@sytue/api-clients';
import type { MatrixBootstrap, SuiteMatrix, StorageRepository } from '@sytue/offline/matrix';
import { decodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import { createPrivateDriveGate } from './private-drive.js';
import {
  privateDriveWriteFixture,
  scope,
  room,
} from '../../../../packages/offline/src/matrix/private-drive-fixture.js';

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
async function fixture() {
  const f = await privateDriveWriteFixture();
  let sourceFactories = 0,
    storageCalls = 0;
  const requests: string[] = [];
  const bootstrap: MatrixBootstrap = {
    actor: scope.subject,
    tenant: scope.tenant,
    matrix_user_id: '@owner:matrix.example',
    homeserver_url: 'https://matrix.example',
    controller_user_id: '@controller:matrix.example',
    sso_provider_id: 'sytue',
    channels: [],
    encryption: 'm.megolm.v1.aes-sha2',
    storage_product: 'drive',
    storage_room_id: room,
    storage_room_ids: [room],
  };
  let storage: () => StorageRepository = () => f.repository();
  // Domain/lifecycle fixture; SDK encryption is covered by the real Matrix interop harness.
  const instance = {
    bootstrap,
    isClosed: false,
    storage: () => {
      storageCalls++;
      return storage();
    },
  } as unknown as SuiteMatrix;
  const source = () => {
    sourceFactories++;
    return new DriveClient({
      baseUrl: 'https://drive.example',
      fetchImpl: Object.assign(
        async (url: Parameters<typeof fetch>[0], init?: Parameters<typeof fetch>[1]) => {
          requests.push(String(url));
          expect(String(url)).toBe('https://drive.example/api/matrix/storage/migration');
          expect(
            decodeStorageScopeHeader(new Headers(init?.headers).get(storageScopeHeader) ?? ''),
          ).toEqual(scope);
          return Response.json(f.state);
        },
        { preconnect: () => {} },
      ),
    });
  };
  const gate = createPrivateDriveGate({ actor: scope.subject, tenant: scope.tenant, source });
  return {
    f,
    instance,
    gate,
    requests,
    counts: () => ({ sourceFactories, storageCalls }),
    setStorage: (factory: () => StorageRepository) => {
      storage = factory;
    },
  };
}

test('Drive gate prepares keys before source access and exposes only a complete private repository', async () => {
  const f = await fixture(),
    keys = deferred(),
    entered = deferred(),
    lifetime = new AbortController();
  f.f.hooks.prepare = async () => {
    entered.resolve();
    await keys.promise;
  };
  const pending = f.gate.host.prepare!(f.instance, lifetime.signal);
  try {
    await entered.promise;
    expect(() => f.gate.current(f.instance)).toThrow('matrix_device');
    expect(f.requests).toHaveLength(0);
    keys.resolve();
    await pending;
    const value = f.gate.current(f.instance);
    expect((await value.repository.root()).id).toBe('root');
    expect(value.storage.scope).toEqual(scope);
    expect(f.requests).toHaveLength(1);
    await f.gate.host.close!(f.instance);
    expect(() => f.gate.current(f.instance)).toThrow('matrix_device');
    expect(() => value.repository.root()).toThrow('closed');
  } finally {
    keys.resolve();
    await pending.catch(() => {});
    lifetime.abort();
    await f.gate.host.close!(f.instance);
    await f.f.close();
  }
});

test('Drive gate rejects another person, tenant or product before acquiring a data source', async () => {
  for (const patch of [
    { actor: 'other' },
    { tenant: 'other' },
    { storage_product: 'mail' as const },
  ]) {
    const f = await fixture();
    Object.assign(f.instance.bootstrap, patch);
    try {
      await expect(f.gate.host.prepare!(f.instance, new AbortController().signal)).rejects.toThrow(
        'matrix_identity',
      );
      expect(f.counts()).toEqual({ sourceFactories: 0, storageCalls: 0 });
      expect(f.requests).toHaveLength(0);
    } finally {
      await f.f.close();
    }
  }
});

test('cancelling Drive during key preparation drains the job and never opens the retained source', async () => {
  const f = await fixture(),
    keys = deferred(),
    entered = deferred(),
    lifetime = new AbortController();
  f.f.hooks.prepare = async () => {
    entered.resolve();
    await keys.promise;
  };
  const pending = f.gate.host.prepare!(f.instance, lifetime.signal).catch(
    (error: unknown) => error,
  );
  let finished = false;
  let closing: Promise<void> | undefined;
  try {
    await entered.promise;
    lifetime.abort();
    closing = f.gate.host.close!(f.instance).then(() => {
      finished = true;
    });
    await new Promise((done) => setTimeout(done, 50));
    expect(finished).toBe(false);
    expect(() => f.gate.current(f.instance)).toThrow('matrix_device');
    keys.resolve();
    await closing;
    expect(await pending).toBeInstanceOf(Error);
    expect(f.requests).toHaveLength(0);
  } finally {
    keys.resolve();
    await pending;
    await closing;
    await f.f.close();
  }
});
