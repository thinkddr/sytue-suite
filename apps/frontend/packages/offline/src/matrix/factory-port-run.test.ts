/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { SuiteMatrix } from './client.js';
import { openSessionOwner } from '../session-owner.js';
import type { HostPortOptions } from '../pqc/host-port.js';

const originalFetch = globalThis.fetch;
const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window');
const originalLocation = Object.getOwnPropertyDescriptor(globalThis, 'location');
afterEach(() => {
  globalThis.fetch = originalFetch;
  for (const [key, descriptor] of [
    ['window', originalWindow],
    ['location', originalLocation],
  ] as const) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor);
    else Reflect.deleteProperty(globalThis, key);
  }
});
const scope = { tenant: 'acme', subject: 'person', product: 'mail' };
type Runner = HostPortOptions['run'];
// Composition witnesses only: no crypto provider, plaintext storage or SDK
// trust claim. The production factory and real WeakMap session owner are used;
// actual Native+Wasm/browser proof remains a separate integration requirement.
async function exerciseFactory(
  historical: boolean,
  exercise: (
    run: Runner,
    state: {
      owner: number;
      current: number;
      historical: number;
      generation: string;
      currentFail: boolean;
      historicalFail: boolean;
    },
  ) => Promise<void>,
) {
  Object.defineProperty(globalThis, 'location', {
    configurable: true,
    value: new URL('https://suite.test/mail/'),
  });
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: { __sytueBase: '/mail' },
  });
  const state = {
    owner: 0,
    current: 0,
    historical: 0,
    generation: 'a'.repeat(64),
    currentFail: false,
    historicalFail: false,
  };
  globalThis.fetch = new Proxy(originalFetch, {
    apply: () => {
      state.owner++;
      return Promise.resolve(
        Response.json({
          authenticated: true,
          user: { id: scope.subject },
          tenant: scope.tenant,
          owner: {
            v: 1,
            actor: scope.subject,
            tenant: scope.tenant,
            client_id: 'mail',
            generation: state.generation,
          },
        }),
      );
    },
  });
  const owner = await openSessionOwner(
    { actor: scope.subject, tenant: scope.tenant, clientID: 'mail' },
    '/mail',
  );
  const lifetime = new AbortController();
  let canonical: HostPortOptions | undefined;
  const handle = (options?: HostPortOptions) => ({
    id: crypto.randomUUID(),
    revoke: () => {},
    drain: async () => {
      await options?.cleanup();
    },
  });
  const kernel = {
    registerHostPort: (options: HostPortOptions) => handle(options),
    invoke: async () => ({ v: 1, scope, max_body_bytes: 1 << 20 }),
  };
  const marker = new Error('composition_complete');
  const domain = {
    registerHostPort: (options: HostPortOptions) => {
      canonical = options;
      return handle(options);
    },
    invoke: async () => {
      expect(canonical).toBeDefined();
      state.owner = state.current = state.historical = 0;
      await exercise(canonical!.run, state);
      throw marker;
    },
  };
  const snapshot = {
    userID: '@isolated:matrix.test',
    deviceID: 'CURRENT',
    rootKey: 'A'.repeat(43),
  };
  const old = Object.assign(Object.create(SuiteMatrix.prototype), {
    network: { lifetime: new AbortController() },
    assertSDKOpen: () => {},
    snapshotSDK: async () => ({ ...snapshot, deviceID: 'ORIGINAL' }),
    revalidateSDK: async () => {
      state.historical++;
      if (state.historicalFail) throw new Error('historical_changed');
    },
    close: () => {},
    closeAndDrain: async () => {},
  });
  const suite = Object.assign(Object.create(SuiteMatrix.prototype), {
    network: { lifetime },
    appOwner: async () => owner,
    vaultScope: () => scope,
    clientCoreRuntime: () => kernel,
    clientModuleRuntime: () => domain,
    snapshotSDK: async () => snapshot,
    revalidateSDK: async () => {
      state.current++;
      if (state.currentFail) throw new Error('current_changed');
    },
    scopedServiceHttp: () => ({ bytes: async () => new Uint8Array() }),
    immutableBlobPort: () => ({}),
    ensureStorageAuthority: async () => {},
    identityCutover: async () => ({ phase: historical ? 'copying' : 'isolated' }),
    identityPlan: async () => ({
      phase: historical ? 'copying' : 'isolated',
      require_legacy: historical,
    }),
    coupleSDKLifetime: () => {},
    storage: () => ({ scope, prepare: async () => {}, assertOpen: () => {} }),
    registerHistoricalStorageReader: async () => handle(),
    close: () => {
      if (!lifetime.signal.aborted) lifetime.abort();
    },
    closeAndDrain: async () => {},
  });
  const create = Reflect.get(SuiteMatrix.prototype, 'createMailPrivate') as (
    ...args: unknown[]
  ) => Promise<unknown>;
  await expect(
    create.call(suite, '/mail', historical ? old : undefined, undefined, undefined),
  ).rejects.toBe(marker);
}
test.each([false, true])(
  'canonical run executes exactly one complete pre/post guard (historical %s)',
  async (historical) => {
    await exerciseFactory(historical, async (run, state) => {
      const order: string[] = [];
      const result = await run(async (signal) => {
        signal.throwIfAborted();
        order.push('action');
        expect(state.owner).toBe(1);
        expect(state.current).toBe(1);
        expect(state.historical).toBe(historical ? 1 : 0);
        return Uint8Array.of(7);
      }, new AbortController().signal);
      expect(result).toEqual(Uint8Array.of(7));
      expect(order).toEqual(['action']);
      expect(state.owner).toBe(2);
      expect(state.current).toBe(2);
      expect(state.historical).toBe(historical ? 2 : 0);
    });
  },
);
test.each(['owner', 'current', 'historical', 'cancel'])(
  'canonical post %s guard erases returned private bytes and rejects',
  async (fault) => {
    await exerciseFactory(true, async (run, state) => {
      const parent = new AbortController(),
        body = Uint8Array.of(7, 8, 9);
      await expect(
        run(async (active) => {
          expect(active.aborted).toBe(false);
          if (fault === 'owner') state.generation = 'b'.repeat(64);
          if (fault === 'current') state.currentFail = true;
          if (fault === 'historical') state.historicalFail = true;
          if (fault === 'cancel') parent.abort();
          return body;
        }, parent.signal),
      ).rejects.toThrow();
      expect([...body]).toEqual([0, 0, 0]);
    });
  },
);
test('canonical pre guard rejects changed historical identity before action', async () => {
  await exerciseFactory(true, async (run, state) => {
    state.historicalFail = true;
    let actions = 0;
    await expect(
      run(async () => {
        actions++;
        return new Uint8Array();
      }, new AbortController().signal),
    ).rejects.toThrow();
    expect(actions).toBe(0);
  });
});
