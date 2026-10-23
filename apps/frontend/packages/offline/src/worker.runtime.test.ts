import { describe, expect, test } from 'bun:test';
import { nativeWorkerResponse } from './worker.js';

const ORIGIN = 'https://suite.invalid';
const api = `${ORIGIN}/mail/api/list`;
const keyOf = (value: Request | string): string =>
  new URL(typeof value === 'string' ? value : value.url, ORIGIN).href;
function deferred<T = void>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
class MemoryCache {
  values = new Map<string, Response>();
  beforePut?: (key: string) => Promise<void>;
  async match(key: Request | string): Promise<Response | undefined> {
    return this.values.get(keyOf(key))?.clone();
  }
  async put(key: Request | string, response: Response): Promise<void> {
    const body = await response.arrayBuffer();
    await this.beforePut?.(keyOf(key));
    this.values.set(
      keyOf(key),
      new Response(body, { status: response.status, headers: response.headers }),
    );
  }
  async delete(key: Request | string): Promise<boolean> {
    return this.values.delete(keyOf(key));
  }
  async keys(): Promise<Request[]> {
    return [...this.values.keys()].map((key) => new Request(key));
  }
}
async function harness(
  workerSource?: string,
  existingCaches?: Map<string, MemoryCache>,
): Promise<{
  caches: Map<string, MemoryCache>;
  initialise: (tenant?: string, scope?: string) => Promise<void>;
  initialiseFields: (fields: Record<string, unknown>) => Promise<void>;
  request: (
    request?: Request,
    clientId?: string,
  ) => { response: Promise<Response>; settled: () => Promise<void> };
  setFetch: (fetcher: (request: Request) => Promise<Response>) => void;
}> {
  const handlers = new Map<string, (event: never) => void>();
  const caches = existingCaches ?? new Map<string, MemoryCache>();
  let fetcher = async (): Promise<Response> => new Response('artificial');
  const storage = {
    async open(name: string) {
      if (!caches.has(name)) caches.set(name, new MemoryCache());
      return caches.get(name)!;
    },
    async keys() {
      return [...caches.keys()];
    },
    async delete(name: string) {
      return caches.delete(name);
    },
  };
  const sw = {
    location: { origin: ORIGIN },
    addEventListener: (name: string, handler: (event: never) => void) =>
      handlers.set(name, handler),
    clients: { matchAll: async () => [], claim: async () => undefined },
    skipWaiting: async () => undefined,
  };
  const indexedDB = {
    deleteDatabase: () => {
      const deletion: { onsuccess?: () => void } = {};
      queueMicrotask(() => deletion.onsuccess?.());
      return deletion;
    },
  };
  new Function(
    'self',
    'caches',
    'fetch',
    'indexedDB',
    workerSource ?? (await nativeWorkerResponse().text()),
  )(sw, storage, (request: Request) => fetcher(request), indexedDB);
  function event(
    type: string,
    values: object,
  ): { response: Promise<Response>; settled: () => Promise<void> } {
    const pending: Promise<unknown>[] = [];
    let response: Promise<Response> | undefined;
    const value = {
      ...values,
      waitUntil: (promise: Promise<unknown>) => {
        pending.push(promise);
      },
      respondWith: (promise: Promise<Response>) => {
        response = Promise.resolve(promise);
      },
    };
    handlers.get(type)!(value as never);
    return {
      response: response ?? fetcher((values as { request: Request }).request),
      settled: async () => {
        while (pending.length) await Promise.all(pending.splice(0));
      },
    };
  }
  async function initialiseFields(fields: Record<string, unknown>): Promise<void> {
    const pending: Promise<unknown>[] = [];
    handlers.get('message')!({
      source: { id: 'window-a' },
      data: {
        type: 'INITIALISE',
        ownerId: 'ada',
        scopeId: 'one',
        product: 'mail',
        ...fields,
      },
      waitUntil: (p: Promise<unknown>) => pending.push(p),
    } as never);
    while (pending.length) await Promise.all(pending.splice(0));
  }
  return {
    caches,
    initialise: (tenant = 'acme', scope = 'one') =>
      initialiseFields({ tenantId: tenant, scopeId: scope }),
    initialiseFields,
    request: (request = new Request(api), clientId = 'window-a') =>
      event('fetch', { request, clientId }),
    setFetch: (value) => {
      fetcher = value;
    },
  };
}
function privateCaches(caches: Map<string, MemoryCache>): MemoryCache[] {
  return [...caches]
    .filter(([key]) => key.includes('-api-') || key.includes('-pages-'))
    .map(([, cache]) => cache);
}
function cachedCount(caches: Map<string, MemoryCache>): number {
  return privateCaches(caches).reduce((sum, cache) => sum + cache.values.size, 0);
}

describe('native worker running its served source', () => {
  test('returns and completes the body before a slow Cache.put, then caches an intact EOF', async () => {
    const app = await harness();
    await app.initialise();
    const gate = deferred();
    // Create the actual API cache before the request, so its persistence can be gated.
    const context = [...app.caches.values()][0]!;
    const value = await (await context.match('/__sytue_offline_context__'))!.json();
    const name = `sytue-native-v2-api-mail-${encodeURIComponent(JSON.stringify([ORIGIN, value.ownerId, value.tenantId]))}`;
    const cache = new MemoryCache();
    cache.beforePut = () => gate.promise;
    app.caches.set(name, cache);
    const work = app.request();
    const response = await work.response;
    expect(await response.text()).toBe('artificial');
    expect(cachedCount(app.caches)).toBe(0);
    gate.resolve();
    await work.settled();
    expect(cachedCount(app.caches)).toBe(1);
    const stored = [...cache.values.values()][0]!;
    expect(stored.headers.get('x-sytue-cache-bytes')).toBe('10');
  });

  test.each(['truncated', 'read-error', 'cancel', 'aborted', 'range', 'set-cookie'])(
    '%s never enters private cache',
    async (kind) => {
      const app = await harness();
      await app.initialise();
      let canceled = false;
      app.setFetch(async () => {
        if (kind === 'read-error')
          return new Response(
            new ReadableStream({
              start(controller) {
                controller.error(new Error('artificial read error'));
              },
            }),
          );
        if (kind === 'cancel')
          return new Response(
            new ReadableStream({
              pull(controller) {
                controller.enqueue(new Uint8Array([1]));
              },
              cancel() {
                canceled = true;
              },
            }),
          );
        return new Response('abc', {
          headers:
            kind === 'truncated'
              ? { 'content-length': '5' }
              : kind === 'set-cookie'
                ? { 'set-cookie': 'nonsecret=artificial' }
                : {},
        });
      });
      const abort = new AbortController();
      const request = new Request(api, {
        signal: abort.signal,
        headers: kind === 'range' ? { range: 'bytes=0-2' } : {},
      });
      const work = app.request(request);
      const response = await work.response;
      if (kind === 'aborted') abort.abort();
      if (kind === 'cancel') {
        await response.body!.cancel();
        expect(canceled).toBe(true);
      } else if (kind === 'read-error')
        await expect(response.text()).rejects.toThrow('artificial read error');
      else expect(await response.text()).toBe('abc');
      await work.settled();
      expect(cachedCount(app.caches)).toBe(0);
    },
  );

  test('unknown length is measured while streaming; oversize body is delivered without persistence', async () => {
    const app = await harness();
    await app.initialise();
    const size = 50 * 1024 * 1024 + 1;
    app.setFetch(async () => {
      let sent = 0;
      return new Response(
        new ReadableStream({
          pull(controller) {
            if (sent === size) {
              controller.close();
              return;
            }
            const n = Math.min(64 * 1024, size - sent);
            sent += n;
            controller.enqueue(new Uint8Array(n));
          },
        }),
      );
    });
    const work = app.request();
    const reader = (await work.response).body!.getReader();
    let received = 0;
    while (true) {
      const part = await reader.read();
      if (part.done) break;
      received += part.value.byteLength;
    }
    expect(received).toBe(size);
    await work.settled();
    expect(cachedCount(app.caches)).toBe(0);
  });

  test('same subject in another tenant cannot commit or retrieve the first tenant response', async () => {
    const app = await harness();
    await app.initialise('acme', 'one');
    const network = deferred<Response>();
    const entered = deferred();
    app.setFetch(() => {
      entered.resolve();
      return network.promise;
    });
    const old = app.request();
    await entered.promise;
    await app.initialise('other', 'two');
    network.resolve(new Response('old organization'));
    expect(await (await old.response).text()).toBe('old organization');
    await old.settled();
    expect(cachedCount(app.caches)).toBe(0);
    app.setFetch(async () => {
      throw new Error('offline');
    });
    await expect(app.request().response).rejects.toThrow('offline');
  });

  test('logout invalidates an in-flight Cache.put and purges it after the write finishes', async () => {
    const app = await harness();
    await app.initialise();
    const context = await (await [...app.caches.values()][0]!.match(
      '/__sytue_offline_context__',
    ))!.json();
    const name = `sytue-native-v2-api-mail-${encodeURIComponent(JSON.stringify([ORIGIN, context.ownerId, context.tenantId]))}`;
    const gate = deferred();
    const entered = deferred();
    const cache = new MemoryCache();
    cache.beforePut = async () => {
      entered.resolve();
      await gate.promise;
    };
    app.caches.set(name, cache);
    const work = app.request();
    const networkResponse = await work.response;
    expect(await networkResponse.clone().text()).toBe('artificial');
    await entered.promise;
    const logout = app.request(new Request(`${ORIGIN}/mail/auth/logout`));
    await logout.response;
    gate.resolve();
    await Promise.all([work.settled(), logout.settled()]);
    await networkResponse.body?.cancel();
    expect(cachedCount(app.caches)).toBe(0);
  });

  test('logout during streaming cannot repopulate the purged cache at a late EOF', async () => {
    const app = await harness();
    await app.initialise();
    let source!: ReadableStreamDefaultController<Uint8Array>;
    app.setFetch(
      async () =>
        new Response(
          new ReadableStream<Uint8Array>({
            start(controller) {
              source = controller;
              controller.enqueue(new Uint8Array([1]));
            },
          }),
        ),
    );
    const work = app.request();
    const reader = (await work.response).body!.getReader();
    expect((await reader.read()).done).toBe(false);
    app.setFetch(async () => new Response('logged out'));
    const logout = app.request(new Request(`${ORIGIN}/mail/auth/logout`));
    await logout.response;
    await logout.settled();
    source.close();
    expect((await reader.read()).done).toBe(true);
    await work.settled();
    expect(cachedCount(app.caches)).toBe(0);
  });

  test('same identity survives a new mount; no-store policy stays deliberate', async () => {
    const app = await harness();
    await app.initialise('acme', 'one');
    app.setFetch(
      async () => new Response('private API', { headers: { 'cache-control': 'no-store' } }),
    );
    const online = app.request();
    await (await online.response).text();
    await online.settled();
    await app.initialise('acme', 'updated-app');
    app.setFetch(async () => {
      throw new Error('offline');
    });
    const offline = app.request();
    const response = await offline.response;
    expect(response.headers.get('x-sytue-offline')).toBe('cache');
    expect(await response.text()).toBe('private API');
    await offline.settled();
  });

  test('one origin budget evicts across product and account caches, leaving unrelated storage alone', async () => {
    const app = await harness();
    await app.initialise();
    const unrelated = new MemoryCache();
    app.caches.set('drafts-queue', unrelated);
    await unrelated.put('/draft', new Response('never evict'));
    for (const [i, suffix] of ['api-other-account', 'pages-other-product', 'static'].entries()) {
      const cache = new MemoryCache();
      app.caches.set(`sytue-native-v2-${suffix}`, cache);
      await cache.put(
        `/old-${i}`,
        new Response('artificial', {
          headers: {
            'x-sytue-cache-bytes': String(30 * 1024 * 1024),
            'x-sytue-cache-written': String(i),
          },
        }),
      );
    }
    const work = app.request();
    await (await work.response).text();
    await work.settled();
    const bytes = [...app.caches]
      .filter(([name]) => name.startsWith('sytue-native-v2-') && !name.endsWith('-meta'))
      .flatMap(([, cache]) => [...cache.values.values()])
      .reduce((sum, response) => sum + Number(response.headers.get('x-sytue-cache-bytes')), 0);
    expect(bytes).toBeLessThanOrEqual(64 * 1024 * 1024);
    expect(await (await unrelated.match('/draft'))!.text()).toBe('never evict');
  });

  test('personal empty tenant recovers persisted cache and remains separate from an organization', async () => {
    const app = await harness();
    await app.initialise('acme', 'organization');
    app.setFetch(async () => new Response('organization data'));
    const org = app.request();
    await (await org.response).text();
    await org.settled();

    await app.initialise('', 'personal');
    app.setFetch(async () => {
      throw new Error('offline');
    });
    await expect(app.request().response).rejects.toThrow('offline');
    app.setFetch(async () => new Response('personal data'));
    const personal = app.request();
    await (await personal.response).text();
    await personal.settled();

    // A restarted worker loads the persisted metadata rather than INITIALISE's
    // in-memory context. The empty tenant must remain valid in both paths.
    const restarted = await harness(undefined, app.caches);
    restarted.setFetch(async () => {
      throw new Error('offline');
    });
    const recovered = restarted.request();
    const response = await recovered.response;
    expect(response.headers.get('x-sytue-offline')).toBe('cache');
    expect(await response.text()).toBe('personal data');
    await recovered.settled();

    await restarted.initialise('acme', 'organization-again');
    await expect(restarted.request().response).rejects.toThrow('offline');
  });

  const invalidTenants: { label: string; fields: Record<string, unknown> }[] = [
    { label: 'absent', fields: {} },
    { label: 'null', fields: { tenantId: null } },
    { label: 'number', fields: { tenantId: 7 } },
    { label: 'boolean', fields: { tenantId: false } },
    { label: 'array', fields: { tenantId: [] } },
    { label: 'object', fields: { tenantId: {} } },
  ];
  test.each(invalidTenants)(
    'INITIALISE rejects invalid tenant schema: $label',
    async ({ fields }) => {
      const app = await harness();
      await app.initialise();
      const online = app.request();
      await (await online.response).text();
      await online.settled();
      await app.initialiseFields(fields);
      const invalidOnline = app.request();
      await (await invalidOnline.response).text();
      await invalidOnline.settled();
      app.setFetch(async () => {
        throw new Error('offline');
      });
      await expect(app.request().response).rejects.toThrow('offline');
    },
  );
  test.each(invalidTenants)(
    'persisted context rejects invalid tenant schema: $label',
    async ({ fields }) => {
      const app = await harness();
      await app.initialise();
      const online = app.request();
      await (await online.response).text();
      await online.settled();
      const metadata = app.caches.get('sytue-native-v2-meta')!;
      const context = await (await metadata.match('/__sytue_offline_context__'))!.json();
      delete context.tenantId;
      await metadata.put(
        '/__sytue_offline_context__',
        new Response(JSON.stringify({ ...context, ...fields })),
      );
      const restarted = await harness(undefined, app.caches);
      const invalidOnline = restarted.request();
      await (await invalidOnline.response).text();
      await invalidOnline.settled();
      restarted.setFetch(async () => {
        throw new Error('offline');
      });
      await expect(restarted.request().response).rejects.toThrow('offline');
    },
  );

  test('Range never falls back to a full previously cached response', async () => {
    const app = await harness();
    await app.initialise();
    const online = app.request();
    await (await online.response).text();
    await online.settled();
    app.setFetch(async () => {
      throw new Error('offline');
    });
    await expect(
      app.request(new Request(api, { headers: { range: 'bytes=0-1' } })).response,
    ).rejects.toThrow('offline');
  });

  test('a new window cannot use another client context before INITIALISE', async () => {
    const app = await harness();
    await app.initialise();
    const online = app.request();
    await (await online.response).text();
    await online.settled();
    app.setFetch(async () => {
      throw new Error('offline');
    });
    await expect(app.request(new Request(api), 'window-b').response).rejects.toThrow('offline');
  });

  test('quota/write failure cannot turn a successful response into an offline fallback', async () => {
    const app = await harness();
    await app.initialise();
    const context = await (await [...app.caches.values()][0]!.match(
      '/__sytue_offline_context__',
    ))!.json();
    const name = `sytue-native-v2-api-mail-${encodeURIComponent(JSON.stringify([ORIGIN, context.ownerId, context.tenantId]))}`;
    const cache = new MemoryCache();
    cache.beforePut = async () => {
      throw new Error('quota');
    };
    app.caches.set(name, cache);
    const work = app.request();
    expect(await (await work.response).text()).toBe('artificial');
    await work.settled();
    expect(cache.values.size).toBe(0);
  });
});
