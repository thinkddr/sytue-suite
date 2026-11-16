/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { MatrixBrowserAuth, type MatrixSessionStore, type MatrixSSOBrowser } from './auth.js';
import { prefijoDeApp } from '@sytue/config/tokens';
import {
  matrixStorePrefix,
  type MatrixBootstrap,
  type MatrixSession,
  type MatrixStorageProduct,
} from './types.js';
const actor = '11111111-1111-4111-8111-111111111111';
const user = `@sytue_${actor}:matrix.test`;
test('private storage SSO fixes the bootstrap organization and person before leaving the app', async () => {
  const privateFlow = fixture('drive');
  await privateFlow.auth.begin(privateFlow.state.bootstrap);
  const target = new URL(privateFlow.state.assigned);
  expect(target.searchParams.getAll('sytue_tenant')).toEqual(['acme']);
  expect(target.searchParams.getAll('sytue_subject')).toEqual([actor]);
  const rooms = fixture();
  await rooms.auth.begin(rooms.state.bootstrap);
  expect(new URL(rooms.state.assigned).searchParams.has('sytue_tenant')).toBe(false);
});
function bootstrap(product?: MatrixStorageProduct): MatrixBootstrap {
  return {
    actor,
    tenant: 'acme',
    homeserver_url: 'https://matrix.test',
    sso_provider_id: 'sytue',
    matrix_user_id: user,
    controller_user_id: '@rooms_controller:matrix.test',
    channels: [],
    encryption: 'm.megolm.v1.aes-sha2',
    ...(product
      ? {
          storage_product: product,
          storage_room_id: '!private:matrix.test',
          storage_room_ids: ['!private:matrix.test'],
        }
      : {}),
  };
}
class Memory implements MatrixSessionStore {
  values = new Map<string, unknown>();
  read = async <T>(owner: string, ns: string, id: string): Promise<T | null> =>
    structuredClone((this.values.get(JSON.stringify([owner, ns, id])) ?? null) as T | null);
  mutate = async <T>(
    owner: string,
    ns: string,
    id: string,
    update: (value: T | null) => T | null,
  ): Promise<void> => {
    const key = JSON.stringify([owner, ns, id]),
      result = update(await this.read<T>(owner, ns, id));
    if (result === null) this.values.delete(key);
    else this.values.set(key, structuredClone(result));
  };
  delete = async (owner: string, ns: string, id: string): Promise<void> => {
    this.values.delete(JSON.stringify([owner, ns, id]));
  };
}
function fixture(product?: MatrixStorageProduct, store = new Memory(), identityVersion?: 1 | 2) {
  const pending = new Map<string, string>();
  const state = {
    bootstrap: bootstrap(product),
    importedAfterCleanup: false,
    imports: 0,
    logins: [] as { token?: string; device_id?: string }[],
    revoked: 0,
    honorDevice: true,
    assigned: '',
    callback: '',
    minted: { user_id: user, device_id: 'PRIVATE_DEVICE', access_token: 'artificial-access-token' },
    afterLogin: undefined as (() => Promise<void>) | undefined,
    acquire:
      async (_prefix: string): Promise<() => void> =>
      () => {},
  };
  if (identityVersion === 2 && product) {
    state.bootstrap.storage_identity_version = 2;
    state.bootstrap.matrix_user_id = `@sytue_storage_${product}_acme_${actor}:matrix.test`;
    state.minted.user_id = state.bootstrap.matrix_user_id;
  }
  const base = prefijoDeApp(product === 'hub' ? 'one' : (product ?? 'rooms'))!;
  const location = {
    href: `https://suite.test${base}/acme/edit?id=opaque`,
    origin: 'https://suite.test',
    pathname: `${base}/acme/edit`,
    search: '?id=opaque',
    assign: (value: string | URL) => {
      state.assigned = String(value);
    },
  };
  const lifecycle = new EventTarget();
  const browser: MatrixSSOBrowser = {
    location,
    lifecycle,
    history: {
      replaceState: (_state, _unused, value) => {
        location.href = new URL(String(value), location.origin).href;
        location.pathname = new URL(location.href).pathname;
        location.search = '';
      },
    },
    sessionStorage: {
      getItem: (key) => pending.get(key) ?? null,
      setItem: (key, value) => {
        pending.set(key, value);
      },
      removeItem: (key) => {
        pending.delete(key);
      },
    },
  };
  const loadClient = async () => {
    state.imports++;
    state.importedAfterCleanup = !new URL(location.href).searchParams.has('loginToken');
    return (options: { baseUrl?: string; accessToken?: string }) => ({
      loginFlows: async () => ({
        flows: [
          { type: 'm.login.sso' as const, identity_providers: [{ id: 'sytue', name: 'Sytue' }] },
        ],
      }),
      getSsoLoginUrl: (callback: string) => {
        state.callback = callback;
        return 'https://matrix.test/artificial-sso';
      },
      loginRequest: async (input: { token?: string; device_id?: string }) => {
        state.logins.push(input);
        if (input.device_id && state.honorDevice) state.minted.device_id = input.device_id;
        await state.afterLogin?.();
        return state.minted;
      },
      logout: async () => {
        expect(options.accessToken).toBe(state.minted.access_token);
        state.revoked++;
        return {};
      },
    });
  };
  const auth = new MatrixBrowserAuth({
    ...(product ? { storageProduct: product } : {}),
    ...(identityVersion !== undefined ? { identityVersion } : {}),
    bootstrap: async () => structuredClone(state.bootstrap),
    deviceName: 'Sytue · Web',
    store,
    browser,
    loadClient,
    acquireDeviceLease: (prefix) => state.acquire(prefix),
  });
  async function start() {
    await auth.begin(state.bootstrap);
    const nonce = new URL(state.callback).searchParams.get('state');
    location.pathname = new URL(state.callback).pathname;
    location.search = `?state=${nonce}&loginToken=artificial-single-use${identityVersion === 2 ? '&identity_version=2' : ''}`;
    location.href = location.origin + location.pathname + location.search;
  }
  return { auth, state, store, pending, location, lifecycle, start, base };
}

test('private SSO removes the one-use URL token before bootstrap, SDK import or durable session writes', async () => {
  const f = fixture('drive');
  await f.start();
  expect(await f.auth.complete()).toBe('/drive/acme/edit?id=opaque');
  expect(f.state.importedAfterCleanup).toBe(true);
  expect(f.pending.size).toBe(0);
  expect(f.state.logins[0]?.token).toBe('artificial-single-use');
  expect(JSON.stringify([...f.store.values])).not.toContain('artificial-single-use');
  const session = await f.auth.readSession(f.state.bootstrap);
  expect(session?.storage_scope).toEqual({ product: 'drive', tenant: 'acme', subject: actor });
  expect(session?.device_id).toBe('PRIVATE_DEVICE');
  await f.start();
  await f.auth.complete();
  expect(f.state.logins[1]?.device_id).toBe('PRIVATE_DEVICE');
});

test('legacy Rooms keeps its deployed namespace, SSO pending key and crypto device across app updates', async () => {
  const f = fixture();
  const session = {
    user_id: user,
    device_id: 'DEPLOYED_DEVICE',
    access_token: 'artificial-old-access',
  };
  f.store.values.set(
    JSON.stringify([user, 'rooms-matrix-session-v1', 'https://matrix.test']),
    session,
  );
  await f.start();
  expect(f.pending.has('sytue-rooms-matrix-sso')).toBe(true);
  expect(await f.auth.complete()).toBe('/rooms/acme/edit?id=opaque');
  expect(f.state.logins[0]?.device_id).toBe('DEPLOYED_DEVICE');
  expect((await f.auth.readSession(f.state.bootstrap))?.storage_scope).toBeUndefined();
});

test('private session identity is bound to product, organisation and actor and never borrows the Rooms namespace', async () => {
  const f = fixture('drive');
  await f.start();
  await f.auth.complete();
  const keys = [...f.store.values.keys()];
  expect(keys[0]).toContain('suite-matrix-session-v1');
  expect(await f.auth.readSession({ ...f.state.bootstrap, tenant: 'other' })).toBeNull();
  await expect(f.auth.readSession(bootstrap('mail'))).rejects.toThrow('matrix_identity');
  const value = [...f.store.values.values()][0] as MatrixSession;
  const originalScope = { ...value.storage_scope! };
  for (const patch of [
    { product: 'mail' as const },
    { subject: 'another' },
    { tenant: 'foreign' },
  ]) {
    value.storage_scope = { ...originalScope, ...patch };
    await expect(f.auth.readSession(f.state.bootstrap)).rejects.toThrow('matrix_identity');
  }
  delete value.storage_scope;
  await expect(f.auth.readSession(f.state.bootstrap)).rejects.toThrow('matrix_identity');
  expect(f.store.values.size).toBe(1);
});

test('SSO rejects changed organisation, product, nonce and open redirects before minting credentials', async () => {
  for (const change of [
    'tenant',
    'product',
    'nonce',
    'return',
    'future',
    'old',
    'duplicate',
    'missing-time',
  ] as const) {
    const f = fixture('drive');
    await f.start();
    const [key, raw] = [...f.pending][0]!,
      pending = JSON.parse(raw);
    if (change === 'tenant') f.state.bootstrap.tenant = 'other';
    if (change === 'product') pending.product = 'mail';
    if (change === 'nonce') pending.nonce = '00000000-0000-4000-8000-000000000000';
    if (change === 'return') pending.returnTo = '/drive/..%2fmail/';
    if (change === 'future') pending.created = Date.now() + 10000;
    if (change === 'old') pending.created = Date.now() - 11 * 60 * 1000;
    if (change === 'missing-time') delete pending.created;
    if (change === 'duplicate') f.location.href += '&loginToken=duplicate';
    f.pending.set(key, JSON.stringify(pending));
    await expect(f.auth.complete()).rejects.toThrow();
    expect(f.state.logins).toHaveLength(0);
    expect(f.pending.size).toBe(0);
    expect(f.location.href).not.toContain('loginToken');
  }
});

test('foreign and late OAuth responses are revoked and never persisted after logout', async () => {
  for (const mode of ['foreign', 'worker-logout', 'local-logout', 'cancel'] as const) {
    const f = fixture('drive');
    await f.start();
    const abort = new AbortController();
    if (mode === 'foreign') f.state.minted.user_id = '@another:matrix.test';
    if (mode === 'worker-logout')
      f.state.afterLogin = async () => {
        f.lifecycle.dispatchEvent(
          new MessageEvent('message', { data: { type: 'SYTUE_CLEAR_OFFLINE' } }),
        );
      };
    if (mode === 'local-logout')
      f.state.afterLogin = async () => {
        await f.auth.forgetSession(f.state.bootstrap);
      };
    if (mode === 'cancel')
      f.state.afterLogin = async () => {
        abort.abort();
      };
    await expect(f.auth.complete(abort.signal)).rejects.toThrow();
    expect(f.state.revoked).toBe(1);
    expect(f.store.values.size).toBe(0);
  }
});

test('logout during commit removes only its own late session and preserves a newer callback session', async () => {
  for (const replace of [false, true]) {
    const f = fixture('drive');
    await f.start();
    const mutate = f.store.mutate;
    let calls = 0;
    f.store.mutate = async (...args) => {
      await mutate(...args);
      if (++calls === 1) {
        if (replace) {
          const key = [...f.store.values.keys()][0]!;
          f.store.values.set(key, {
            ...f.state.minted,
            access_token: 'artificial-newer-access',
            storage_scope: { product: 'drive', tenant: 'acme', subject: actor },
          });
        }
        f.lifecycle.dispatchEvent(
          new MessageEvent('message', { data: { type: 'SYTUE_CLEAR_OFFLINE' } }),
        );
      }
    };
    await expect(f.auth.complete()).rejects.toThrow('matrix_device');
    expect(f.state.revoked).toBe(1);
    expect([...f.store.values.values()].length).toBe(replace ? 1 : 0);
    if (replace)
      expect((await f.auth.readSession(f.state.bootstrap))?.access_token).toBe(
        'artificial-newer-access',
      );
  }
});

test('callback token is cleared even when pending state is malformed or already consumed', async () => {
  const f = fixture('drive');
  await f.start();
  const key = [...f.pending.keys()][0]!;
  f.pending.set(key, '{');
  await expect(f.auth.complete()).rejects.toThrow('matrix_identity');
  expect(f.location.href).not.toContain('loginToken');
  expect(f.state.logins).toHaveLength(0);
  await expect(f.auth.complete()).rejects.toThrow('matrix_login');
});

test('a renewed login cannot silently replace a requested deployed crypto device', async () => {
  const f = fixture();
  const previous = {
    user_id: user,
    device_id: 'DEPLOYED_DEVICE',
    access_token: 'artificial-existing-access',
  };
  f.store.values.set(
    JSON.stringify([user, 'rooms-matrix-session-v1', 'https://matrix.test']),
    previous,
  );
  f.state.honorDevice = false;
  await f.start();
  await expect(f.auth.complete()).rejects.toThrow('matrix_device');
  expect(f.state.revoked).toBe(1);
  expect(await f.auth.readSession(f.state.bootstrap)).toEqual(previous);
});

test('another callback cannot overwrite a newer account session while exchanging its token', async () => {
  const f = fixture('drive');
  await f.start();
  const newer = {
    ...f.state.minted,
    access_token: 'artificial-newer-access',
    storage_scope: { product: 'drive' as const, tenant: 'acme', subject: actor },
  };
  f.state.afterLogin = async () => {
    const owner = JSON.stringify(['https://matrix.test', user, 'acme', actor, 'drive']);
    f.store.values.set(
      JSON.stringify([owner, 'suite-matrix-session-v1', 'https://matrix.test']),
      newer,
    );
  };
  await expect(f.auth.complete()).rejects.toThrow('matrix_locked');
  expect(f.state.revoked).toBe(1);
  expect(await f.auth.readSession(f.state.bootstrap)).toEqual(newer);
});

test('all fifteen private products keep independent sessions and fixed callbacks on their real app prefixes', async () => {
  const store = new Memory();
  const products: MatrixStorageProduct[] = [
    'hub',
    'mail',
    'drive',
    'docs',
    'sheets',
    'slides',
    'pdf',
    'calendar',
    'contacts',
    'notes',
    'photos',
    'rooms',
    'cast',
    'erp',
    'bases',
  ];
  for (const product of products) {
    const f = fixture(product, store);
    await f.start();
    expect(new URL(f.state.callback).pathname).toBe(f.base + '/auth/matrix/callback');
    expect(await f.auth.complete()).toBe(f.base + '/acme/edit?id=opaque');
    expect((await f.auth.readSession(f.state.bootstrap))?.storage_scope?.product).toBe(product);
  }
  expect(store.values.size).toBe(15);
  expect(await fixture(undefined, store).auth.readSession(bootstrap())).toBeNull();
});

test('default bootstrap stays on the current app BFF, with cookies and no HTTP cache', async () => {
  const original = globalThis.fetch;
  const requests: { path: string; options?: RequestInit }[] = [];
  let product: MatrixStorageProduct | undefined;
  globalThis.fetch = (async (input, options) => {
    requests.push({ path: String(input), ...(options ? { options } : {}) });
    return Response.json(bootstrap(product));
  }) as typeof fetch;
  try {
    for (product of [undefined, 'hub', 'docs', 'rooms', 'drive'] as const) {
      const auth = new MatrixBrowserAuth({
        ...(product ? { storageProduct: product } : {}),
        deviceName: 'Artificial',
      });
      await auth.refreshBootstrap();
      const base = prefijoDeApp(product === 'hub' ? 'one' : (product ?? 'rooms'));
      const request = requests.at(-1)!;
      expect(request.path).toBe(
        base + (product ? '/api/matrix/storage/bootstrap' : '/api/matrix/bootstrap'),
      );
      expect(request.options?.credentials).toBe('same-origin');
      expect(request.options?.cache).toBe('no-store');
    }
  } finally {
    globalThis.fetch = original;
  }
});

test('SSO never reuses a device while its crypto store is still owned or being erased', async () => {
  const f = fixture('drive');
  await f.start();
  await f.auth.complete();
  const retained = await f.auth.readSession(f.state.bootstrap);
  f.state.acquire = async () => {
    throw new Error('matrix_locked');
  };
  await f.start();
  await expect(f.auth.complete()).rejects.toThrow('matrix_locked');
  expect(f.state.logins).toHaveLength(1);
  expect(f.state.revoked).toBe(0);
  expect(await f.auth.readSession(f.state.bootstrap)).toEqual(retained);
  expect(f.location.href).not.toContain('loginToken');
});

test('SSO retains the device lease until the replacement session is committed', async () => {
  const f = fixture('drive');
  await f.start();
  await f.auth.complete();
  let acquired = false,
    released = false,
    committed = false;
  f.state.acquire = async (prefix) => {
    expect(prefix.startsWith('sytue-storage-matrix-')).toBe(true);
    acquired = true;
    return () => {
      released = true;
    };
  };
  const mutate = f.store.mutate;
  f.store.mutate = async (...args) => {
    expect(acquired).toBe(true);
    expect(released).toBe(false);
    await mutate(...args);
    committed = true;
  };
  await f.start();
  await f.auth.complete();
  expect(committed).toBe(true);
  expect(released).toBe(true);
});

test('an old client forgetting its captured session cannot erase a newer login', async () => {
  const f = fixture('drive');
  await f.start();
  await f.auth.complete();
  const previous = (await f.auth.readSession(f.state.bootstrap))!;
  const newer = { ...previous, device_id: 'NEW_DEVICE', access_token: 'artificial-newer-session' };
  const key = [...f.store.values.keys()][0]!;
  f.store.values.set(key, newer);
  await f.auth.forgetSession(f.state.bootstrap, previous);
  expect(await f.auth.readSession(f.state.bootstrap)).toEqual(newer);
  await f.auth.forgetSession(f.state.bootstrap, newer);
  expect(await f.auth.readSession(f.state.bootstrap)).toBeNull();
});

test('crypto-store names preserve deployed legacy Rooms and private device data', () => {
  expect(matrixStorePrefix(bootstrap(), 'PRIVATE_DEVICE')).toBe(
    'sytue-rooms-matrix-https%3A%2F%2Fmatrix.test-%40sytue_11111111-1111-4111-8111-111111111111%3Amatrix.test-PRIVATE_DEVICE',
  );
  expect(matrixStorePrefix(bootstrap('drive'), 'PRIVATE_DEVICE')).toBe(
    'sytue-storage-matrix-%5B%22drive%22%2C%22acme%22%2C%2211111111-1111-4111-8111-111111111111%22%2C%22https%3A%2F%2Fmatrix.test%22%2C%22%40sytue_11111111-1111-4111-8111-111111111111%3Amatrix.test%22%2C%22PRIVATE_DEVICE%22%5D',
  );
});

test('isolated storage SSO preserves exact identity version, nonce, user and separate pending namespace', async () => {
  const f = fixture('mail', new Memory(), 2);
  await f.start();
  expect(f.pending.has('sytue-storage-matrix-sso-mail-v2')).toBe(true);
  expect(new URL(f.state.callback).searchParams.getAll('identity_version')).toEqual(['2']);
  expect(await f.auth.complete()).toBe('/mail/acme/edit?id=opaque');
  expect((await f.auth.readSession(f.state.bootstrap))?.user_id).toBe(
    f.state.bootstrap.matrix_user_id,
  );
  for (const change of ['bootstrap', 'pending', 'missing', 'duplicate', 'legacy'] as const) {
    const wrong = fixture('mail', new Memory(), 2);
    await wrong.start();
    if (change === 'bootstrap') wrong.state.bootstrap = bootstrap('mail');
    if (change === 'pending') {
      const [key, raw] = [...wrong.pending][0]!;
      wrong.pending.set(key, JSON.stringify({ ...JSON.parse(raw), identityVersion: 1 }));
    }
    if (change === 'missing')
      wrong.location.href = wrong.location.href.replace('&identity_version=2', '');
    if (change === 'duplicate') wrong.location.href += '&identity_version=2';
    if (change === 'legacy')
      wrong.location.href = wrong.location.href.replace('identity_version=2', 'identity_version=1');
    await expect(wrong.auth.complete()).rejects.toThrow('matrix_');
    expect(wrong.state.logins).toHaveLength(0);
    expect(wrong.location.href).not.toContain('loginToken');
  }
  const legacy = fixture('mail');
  await legacy.start();
  expect(legacy.pending.has('sytue-storage-matrix-sso-mail')).toBe(true);
  legacy.state.bootstrap = f.state.bootstrap;
  await expect(legacy.auth.complete()).rejects.toThrow('matrix_identity');
  expect(legacy.state.logins).toHaveLength(0);
});
