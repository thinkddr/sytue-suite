/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { SuiteMatrix } from './client.js';
import { openSessionOwner, type OpaqueSessionOwner } from '../session-owner.js';
import { sessionOwnerHeader } from '@sytue/config/session-owner';
import {
  encodeStorageScopeHeader,
  storageScopeHeader,
  type MatrixStorageScope,
} from '@sytue/config/matrix';
import type { StorageBlobPort } from './storage-files.js';

const originalFetch = globalThis.fetch,
  originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window'),
  originalLocation = Object.getOwnPropertyDescriptor(globalThis, 'location');
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
const scope: MatrixStorageScope = { tenant: 'acme', subject: 'person', product: 'mail' };
const descriptor = {
  v: 1,
  actor: scope.subject,
  tenant: scope.tenant,
  client_id: 'mail',
  generation: 'a'.repeat(64),
};
const blobID = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';

test.each([
  { build: '/mail', host: '/mail', prefix: '/mail' },
  { build: '/built-elsewhere', host: '/mail', prefix: '/mail' },
  { build: '/', host: '/', prefix: '' },
])(
  'cipher blob factory keeps host $host and its actual opaque BFF owner',
  async ({ build, host, prefix }) => {
    Object.defineProperty(globalThis, 'location', {
      configurable: true,
      value: new URL('https://suite.test' + prefix + '/'),
    });
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: { __sytueBase: host },
    });
    const seen: Request[] = [],
      options: (RequestInit | undefined)[] = [];
    globalThis.fetch = new Proxy(originalFetch, {
      apply: (_target, _receiver, args) => {
        const request = new Request(...(args as Parameters<typeof fetch>));
        seen.push(request);
        options.push(args[1] as RequestInit | undefined);
        if (new URL(request.url).pathname === prefix + '/auth/session')
          return Promise.resolve(
            Response.json({
              authenticated: true,
              user: { id: scope.subject },
              tenant: scope.tenant,
              owner: descriptor,
            }),
          );
        return Promise.resolve(new Response(Uint8Array.of(1, 2, 3)));
      },
    });
    // The production session API creates the real WeakMap capability. No copied
    // owner object or caller-supplied descriptor is accepted by the factory.
    const owner = await openSessionOwner(
      { actor: scope.subject, tenant: scope.tenant, clientID: 'mail' },
      build,
    );
    try {
      const create = Reflect.get(SuiteMatrix.prototype, 'immutableBlobPort') as (
        this: { vaultScope: () => typeof scope },
        owner: OpaqueSessionOwner,
        base: string,
      ) => StorageBlobPort;
      const context = { vaultScope: () => scope };
      const blobs = create.call(context, owner, build);
      expect(await blobs.read(blobID)).toEqual(Uint8Array.of(1, 2, 3));
      expect(seen.map((request) => request.url)).toEqual([
        'https://suite.test' + prefix + '/auth/session',
        'https://suite.test' + prefix + '/api/matrix/storage/blobs/mail/' + blobID + '/content',
      ]);
      const fetched = seen[1]!;
      expect(fetched.headers.get(sessionOwnerHeader)).toBe(descriptor.generation);
      expect(fetched.headers.get(storageScopeHeader)).toBe(encodeStorageScopeHeader(scope));
      expect(options[1]?.credentials).toBe('same-origin');
      expect(fetched.redirect).toBe('error');
      expect(fetched.headers.has('Authorization')).toBe(false);
      expect(() => create.call(context, { ...owner }, build)).toThrow('session_owner_mismatch');
      expect(seen).toHaveLength(2);
    } finally {
      await owner.closeAndDrain();
    }
  },
);
