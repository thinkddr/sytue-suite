/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { ExternalE2EEKeyProvider } from 'livekit-client';
import { CallEncryption } from './callEncryption.js';
import { RoomsMatrix, setRoomsMatrix, type MatrixCallKeyContent } from './matrix/client.js';
import type { MatrixCallScope } from './matrix/types.js';

const actor = '11111111-1111-4111-8111-111111111111';
const other = '22222222-2222-4222-8222-222222222222';
const device = 'ARTIFICIAL_LOCAL';
const call = '33333333-3333-4333-8333-333333333333';
const join = '44444444-4444-4444-8444-444444444444';
function binding(): MatrixCallScope {
  return {
    call_id: call,
    matrix_room_id: `!${'a'.repeat(43)}`,
    livekit_room: 'artificial-generation-1',
    generation: 1,
    expires_at_ms: Date.now() + 60000,
    leader_subject: actor,
    leader_device_id: device,
    members: [
      { subject: actor, device_id: device },
      { subject: other, device_id: 'ARTIFICIAL_REMOTE' },
    ],
  };
}
function matrix(
  trace: string[],
  automaticKey = true,
): {
  receive: (content: MatrixCallKeyContent) => void;
  requested: Promise<void>;
} {
  let receive: ((content: MatrixCallKeyContent) => void) | undefined;
  let signalRequest!: () => void;
  const requested = new Promise<void>((resolve) => {
    signalRequest = resolve;
  });
  const instance = Object.assign(Object.create(RoomsMatrix.prototype), {
    bootstrap: { actor, homeserver_url: 'https://matrix.test' },
    client: { getDeviceId: () => device },
    prepareCallScope: async () => undefined,
    onEncryptedCustom: (
      _scope: MatrixCallScope,
      type: string,
      callback: (content: MatrixCallKeyContent) => void,
    ) => {
      if (type.endsWith('call_key.v1')) receive = callback;
      return () => {
        receive = undefined;
      };
    },
    sendEncryptedCustom: async (scope: MatrixCallScope, type: string) => {
      trace.push(type.endsWith('request.v1') ? 'request-key' : 'share-key');
      if (type.endsWith('request.v1')) signalRequest();
      if (automaticKey && type.endsWith('request.v1'))
        queueMicrotask(() =>
          receive?.({
            v: 1,
            call_id: call,
            generation: scope.generation,
            algorithm: 'AES-GCM',
            expires_at: scope.expires_at_ms,
            key_b64: 'B'.repeat(43) + '=',
          }),
        );
    },
  }) as RoomsMatrix;
  setRoomsMatrix(instance);
  return { receive: (content) => receive?.(content), requested };
}
function connection(scope: MatrixCallScope) {
  return {
    identity: actor,
    joinId: join,
    encryption: scope,
    room: scope.livekit_room,
    serverUrl: 'wss://artificial.invalid',
    token: 'artificial-test-only',
  };
}

test('call publishing waits for key installation and server ready; a new epoch reconnects before publishing', async () => {
  const trace: string[] = [];
  matrix(trace);
  const previousFetch = globalThis.fetch;
  let scope = binding();
  let readyCalls = 0;
  let denied = false;
  const provider = new ExternalE2EEKeyProvider();
  const setKey = provider.setKey.bind(provider);
  provider.setKey = async (key: string | ArrayBuffer) => {
    trace.push('install-key');
    await setKey(key);
  };
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (denied) return new Response('{}', { status: 403 });
    if (!init?.body) return Response.json(scope);
    const body = JSON.parse(String(init.body)) as { action: string };
    if (body.action === 'ready') {
      readyCalls++;
      trace.push('server-ready');
      return Response.json({});
    }
    if (body.action === 'reconnect') return Response.json(connection(scope));
    throw new Error('Unexpected artificial request');
  }) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(call, connection(scope), provider, {
      pause: async () => {
        trace.push('pause');
      },
      connect: async () => {
        trace.push('connect');
      },
      ready: async () => {
        trace.push('publish');
      },
      failed: () => {
        trace.push('failed');
      },
    });
    expect(trace).toEqual(['pause', 'install-key', 'share-key']);
    expect(readyCalls).toBe(0);
    expect(encryption.ready).toBe(false);
    await encryption.started();
    expect(trace.slice(-2)).toEqual(['server-ready', 'publish']);
    expect(encryption.ready).toBe(true);
    trace.length = 0;
    scope = {
      ...scope,
      generation: 2,
      matrix_room_id: `!${'b'.repeat(43)}`,
      livekit_room: 'artificial-generation-2',
      leader_subject: other,
      leader_device_id: 'ARTIFICIAL_REMOTE',
    };
    await encryption.refresh();
    expect(trace).toEqual([
      'pause',
      'request-key',
      'install-key',
      'connect',
      'server-ready',
      'publish',
    ]);
    denied = true;
    await expect(encryption.refresh()).rejects.toThrow();
    expect(encryption.ready).toBe(false);
    expect(trace.at(-1)).toBe('failed');
  } finally {
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test('a short permission lease stops a call at its expiry without waiting for the polling interval', async () => {
  const trace: string[] = [];
  matrix(trace);
  const scope = { ...binding(), expires_at_ms: Date.now() + 200 };
  const encryption = await CallEncryption.prepare(
    call,
    connection(scope),
    new ExternalE2EEKeyProvider(),
    {
      pause: async () => undefined,
      connect: async () => undefined,
      ready: async () => undefined,
      failed: () => {
        trace.push('expired');
      },
    },
  );
  try {
    await new Promise((resolve) => setTimeout(resolve, 250));
    expect(trace).toEqual(['share-key', 'expired']);
    expect(encryption.ready).toBe(false);
  } finally {
    encryption.close();
    setRoomsMatrix(null);
  }
});

test('concurrent conflicting keys fail closed while the SDK is still installing the first', async () => {
  const trace: string[] = [];
  const delivery = matrix(trace, false);
  const scope = {
    ...binding(),
    leader_subject: other,
    leader_device_id: 'ARTIFICIAL_REMOTE',
  };
  let release!: () => void;
  let entered!: () => void;
  const installed = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const provider = new ExternalE2EEKeyProvider();
  provider.setKey = async () => {
    trace.push('install-start');
    entered();
    await gate;
    trace.push('install-finish');
  };
  const pending = CallEncryption.prepare(call, connection(scope), provider, {
    pause: async () => undefined,
    connect: async () => undefined,
    ready: async () => {
      trace.push('publish');
    },
    failed: () => {
      trace.push('failed');
    },
  });
  const settled = pending.then(
    () => 'ready',
    () => 'rejected',
  );
  const key: MatrixCallKeyContent = {
    v: 1,
    call_id: call,
    generation: scope.generation,
    algorithm: 'AES-GCM',
    expires_at: scope.expires_at_ms,
    key_b64: 'A'.repeat(43) + '=',
  };
  try {
    await delivery.requested;
    delivery.receive(key);
    await installed;
    delivery.receive({ ...key, key_b64: 'B'.repeat(43) + '=' });
    await Promise.resolve();
    expect(trace).toEqual(['request-key', 'install-start', 'failed']);
    expect(await settled).toBe('rejected');
    release();
    await Promise.resolve();
    expect(trace).not.toContain('publish');
  } finally {
    release();
    setRoomsMatrix(null);
  }
});

test('reconnect rejects a response that changes the checked epoch before connecting', async () => {
  const trace: string[] = [];
  matrix(trace);
  const scope = binding();
  const previousFetch = globalThis.fetch;
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.body) return Response.json(scope);
    const body = JSON.parse(String(init.body)) as { action: string };
    if (body.action === 'ready') return Response.json({});
    if (body.action === 'reconnect')
      return Response.json({
        ...connection(scope),
        encryption: { ...scope, expires_at_ms: scope.expires_at_ms - 1000 },
      });
    throw new Error('Unexpected artificial request');
  }) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(
      call,
      connection(scope),
      new ExternalE2EEKeyProvider(),
      {
        pause: async () => undefined,
        connect: async () => {
          trace.push('connect');
        },
        ready: async () => undefined,
        failed: () => trace.push('failed'),
      },
    );
    await encryption.started();
    await expect(encryption.refresh(true)).rejects.toThrow();
    expect(trace).not.toContain('connect');
    expect(trace.at(-1)).toBe('failed');
    expect(encryption.ready).toBe(false);
  } finally {
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test('an epoch cannot change its roster, leader, deadline, or transport without rotation', async () => {
  const variants: Array<(scope: MatrixCallScope) => MatrixCallScope> = [
    (scope) => ({ ...scope, members: [scope.members[0]!] }),
    (scope) => ({ ...scope, leader_subject: other, leader_device_id: 'ARTIFICIAL_REMOTE' }),
    (scope) => ({ ...scope, expires_at_ms: scope.expires_at_ms - 1000 }),
    (scope) => ({ ...scope, matrix_room_id: `!${'b'.repeat(43)}` }),
    (scope) => ({ ...scope, livekit_room: 'altered-transport' }),
    (scope) => ({ ...scope, generation: scope.generation + 1 }),
    (scope) => {
      scope.members[1]!.device_id = 'REPLACED_REMOTE';
      return scope;
    },
  ];
  for (const mutate of variants) {
    const trace: string[] = [];
    matrix(trace);
    const original = binding();
    let response = original;
    const previousFetch = globalThis.fetch;
    globalThis.fetch = (async (_input: RequestInfo | URL) =>
      Response.json(response)) as typeof fetch;
    let encryption: CallEncryption | undefined;
    try {
      encryption = await CallEncryption.prepare(
        call,
        connection(original),
        new ExternalE2EEKeyProvider(),
        {
          pause: async () => undefined,
          connect: async () => undefined,
          ready: async () => undefined,
          failed: () => trace.push('failed'),
        },
      );
      response = mutate(original);
      await expect(encryption.refresh()).rejects.toThrow();
      expect(trace.at(-1)).toBe('failed');
      expect(encryption.ready).toBe(false);
    } finally {
      encryption?.close();
      globalThis.fetch = previousFetch;
      setRoomsMatrix(null);
    }
  }
});

test('the same roster in a different order does not spuriously rotate an epoch', async () => {
  const trace: string[] = [];
  matrix(trace);
  const scope = binding();
  const previousFetch = globalThis.fetch;
  globalThis.fetch = (async (_input: RequestInfo | URL) =>
    Response.json({ ...scope, members: [...scope.members].reverse() })) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(
      call,
      connection(scope),
      new ExternalE2EEKeyProvider(),
      {
        pause: async () => {
          trace.push('pause');
        },
        connect: async () => {
          trace.push('connect');
        },
        ready: async () => {
          trace.push('publish');
        },
        failed: () => trace.push('failed'),
      },
    );
    await encryption.refresh();
    expect(trace).toEqual(['pause', 'share-key']);
  } finally {
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test.each([false, true])(
  'a temporary reconnect failure is retried without publishing (rotation %s)',
  async (rotate) => {
    const trace: string[] = [];
    matrix(trace);
    let scope = binding();
    let reconnectAttempts = 0;
    const previousFetch = globalThis.fetch;
    globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (!init?.body) return Response.json(scope);
      const body = JSON.parse(String(init.body)) as { action: string };
      if (body.action === 'ready') {
        trace.push('server-ready');
        return Response.json({});
      }
      if (body.action === 'reconnect') {
        trace.push('reconnect-request');
        if (++reconnectAttempts === 1) return new Response('{}', { status: 503 });
        return Response.json(connection(scope));
      }
      throw new Error('Unexpected artificial request');
    }) as typeof fetch;
    let encryption: CallEncryption | undefined;
    try {
      encryption = await CallEncryption.prepare(
        call,
        connection(scope),
        new ExternalE2EEKeyProvider(),
        {
          pause: async () => {
            trace.push('pause');
          },
          connect: async () => {
            trace.push('connect');
          },
          ready: async () => {
            trace.push('publish');
          },
          failed: () => trace.push('failed'),
        },
      );
      await encryption.started();
      trace.length = 0;
      if (rotate)
        scope = {
          ...scope,
          generation: 2,
          matrix_room_id: `!${'b'.repeat(43)}`,
          livekit_room: 'artificial-generation-2',
        };
      await encryption.refresh(!rotate);
      expect(encryption.ready).toBe(false);
      expect(trace).not.toContain('connect');
      expect(trace).not.toContain('publish');
      await Promise.resolve();
      await Promise.resolve();
      expect(reconnectAttempts).toBe(1); // the scheduled retry is not a hot loop
      await encryption.refresh();
      expect(reconnectAttempts).toBe(2);
      expect(trace.slice(-3)).toEqual(['connect', 'server-ready', 'publish']);
      expect(encryption.ready).toBe(true);
      expect(encryption.publicationGuard()()).toBe(true);
    } finally {
      encryption?.close();
      globalThis.fetch = previousFetch;
      setRoomsMatrix(null);
    }
  },
);

test('an old ready response cannot authorize media after a reconnect was requested', async () => {
  const trace: string[] = [];
  matrix(trace);
  const scope = binding();
  let release!: () => void;
  let entered!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const waiting = new Promise<void>((resolve) => {
    entered = resolve;
  });
  let readyCalls = 0;
  const previousFetch = globalThis.fetch;
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.body) return Response.json(scope);
    const body = JSON.parse(String(init.body)) as { action: string };
    if (body.action === 'ready') {
      if (++readyCalls === 1) {
        entered();
        await gate;
      }
      return Response.json({});
    }
    if (body.action === 'reconnect') return Response.json(connection(scope));
    throw new Error('Unexpected artificial request');
  }) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(
      call,
      connection(scope),
      new ExternalE2EEKeyProvider(),
      {
        pause: async () => undefined,
        connect: async () => {
          trace.push('connect');
        },
        ready: async () => {
          trace.push('publish');
        },
        failed: () => trace.push('failed'),
      },
    );
    const starting = encryption.started();
    await waiting;
    const reconnecting = encryption.refresh(true);
    release();
    await Promise.all([starting, reconnecting]);
    expect(encryption.ready).toBe(false);
    expect(trace).not.toContain('publish');
    await encryption.refresh();
    expect(trace).toEqual(['share-key', 'connect', 'publish']);
    expect(encryption.ready).toBe(true);
  } finally {
    release();
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test('a reconnected transport cannot reactivate media before the new ready acknowledgement', async () => {
  const trace: string[] = [];
  matrix(trace);
  const scope = binding();
  let release!: () => void;
  let entered!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const waiting = new Promise<void>((resolve) => {
    entered = resolve;
  });
  let readyCalls = 0;
  const previousFetch = globalThis.fetch;
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.body) return Response.json(scope);
    const body = JSON.parse(String(init.body)) as { action: string };
    if (body.action === 'ready') {
      if (++readyCalls === 2) {
        entered();
        await gate;
      }
      return Response.json({});
    }
    if (body.action === 'reconnect') return Response.json(connection(scope));
    throw new Error('Unexpected artificial request');
  }) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(
      call,
      connection(scope),
      new ExternalE2EEKeyProvider(),
      {
        pause: async () => undefined,
        connect: async () => {
          trace.push('connect');
        },
        ready: async () => {
          trace.push('publish');
        },
        failed: () => trace.push('failed'),
      },
    );
    await encryption.started();
    const oldPermit = encryption.publicationGuard();
    encryption.suspendForReconnect();
    trace.length = 0;
    const reconnecting = encryption.refresh();
    await waiting;
    expect(trace).toEqual(['connect']);
    expect(encryption.ready).toBe(false);
    expect(oldPermit()).toBe(false);
    release();
    await reconnecting;
    expect(trace).toEqual(['connect', 'publish']);
    expect(encryption.ready).toBe(true);
    expect(oldPermit()).toBe(false);
  } finally {
    release();
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test('a suspended permit stays invalid until an explicit reconnect and new acknowledgement', async () => {
  const trace: string[] = [];
  matrix(trace);
  const scope = binding();
  const previousFetch = globalThis.fetch;
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.body) return Response.json(scope);
    const body = JSON.parse(String(init.body)) as { action: string };
    if (body.action === 'ready') return Response.json({});
    if (body.action === 'reconnect') return Response.json(connection(scope));
    throw new Error('Unexpected artificial request');
  }) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(
      call,
      connection(scope),
      new ExternalE2EEKeyProvider(),
      {
        pause: async () => undefined,
        connect: async () => {
          trace.push('connect');
        },
        ready: async () => {
          trace.push('publish');
        },
        failed: () => trace.push('failed'),
      },
    );
    await encryption.started();
    const oldPermit = encryption.publicationGuard();
    expect(oldPermit()).toBe(true);
    encryption.suspendForReconnect();
    expect(oldPermit()).toBe(false);
    expect(encryption.ready).toBe(false);
    trace.length = 0;
    await encryption.refresh();
    expect(trace).toEqual(['connect', 'publish']);
    expect(encryption.ready).toBe(true);
    expect(oldPermit()).toBe(false);
  } finally {
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test('a rotated epoch installs a fresh key and permanently revokes the prior permit', async () => {
  const trace: string[] = [];
  matrix(trace);
  let scope = binding();
  const keys: (string | ArrayBuffer)[] = [];
  const provider = new ExternalE2EEKeyProvider();
  provider.setKey = async (key: string | ArrayBuffer) => {
    keys.push(key);
  };
  const previousFetch = globalThis.fetch;
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.body) return Response.json(scope);
    const body = JSON.parse(String(init.body)) as { action: string };
    if (body.action === 'ready') return Response.json({});
    if (body.action === 'reconnect') return Response.json(connection(scope));
    throw new Error('Unexpected artificial request');
  }) as typeof fetch;
  let encryption: CallEncryption | undefined;
  try {
    encryption = await CallEncryption.prepare(call, connection(scope), provider, {
      pause: async () => undefined,
      connect: async () => undefined,
      ready: async () => undefined,
      failed: () => trace.push('failed'),
    });
    await encryption.started();
    const oldPermit = encryption.publicationGuard();
    scope = {
      ...scope,
      generation: 2,
      matrix_room_id: `!${'b'.repeat(43)}`,
      livekit_room: 'artificial-generation-2',
    };
    await encryption.refresh();
    expect(keys).toHaveLength(2);
    expect(keys[0]).not.toEqual(keys[1]);
    expect(oldPermit()).toBe(false);
    expect(encryption.publicationGuard()()).toBe(true);
    encryption.close();
    expect(encryption.ready).toBe(false);
    expect(oldPermit()).toBe(false);
  } finally {
    encryption?.close();
    globalThis.fetch = previousFetch;
    setRoomsMatrix(null);
  }
});

test('an expired authorization cancels a pending key request and ignores late keys', async () => {
  const trace: string[] = [];
  const delivery = matrix(trace, false);
  const scope = {
    ...binding(),
    expires_at_ms: Date.now() + 200,
    leader_subject: other,
    leader_device_id: 'ARTIFICIAL_REMOTE',
  };
  const provider = new ExternalE2EEKeyProvider();
  provider.setKey = async () => {
    trace.push('install-key');
  };
  try {
    const pending = CallEncryption.prepare(call, connection(scope), provider, {
      pause: async () => undefined,
      connect: async () => undefined,
      ready: async () => {
        trace.push('publish');
      },
      failed: () => trace.push('failed'),
    });
    await delivery.requested;
    await expect(pending).rejects.toThrow();
    delivery.receive({
      v: 1,
      call_id: call,
      generation: scope.generation,
      algorithm: 'AES-GCM',
      expires_at: scope.expires_at_ms,
      key_b64: 'A'.repeat(43) + '=',
    });
    await Promise.resolve();
    expect(trace).toEqual(['request-key', 'failed']);
  } finally {
    setRoomsMatrix(null);
  }
});
