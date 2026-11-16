/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { SuiteMatrix } from './client.js';
import { StorageJobs } from './storage-jobs.js';
import { recoveryUILifetime } from './recovery-ui.js';

test.each(['owner-key', 'uploads', 'session', 'sdk'])(
  'a failing %s cleanup cannot prevent erasing the other private credentials',
  async (failure) => {
    const erased: string[] = [];
    const erase = async (part: string) => {
      erased.push(part);
      if (part === failure) throw new Error('artificial cleanup failure');
    };
    const instance = Object.assign(Object.create(SuiteMatrix.prototype), {
      close: () => {
        erased.push('closed');
      },
      release: () => {},
      ownerKeys: { destroy: () => erase('owner-key') },
      uploadCache: { destroy: () => erase('uploads') },
      adapter: { forgetSession: () => erase('session') },
      client: { clearStores: () => erase('sdk') },
    }) as SuiteMatrix;
    await expect(instance.forget()).rejects.toThrow('matrix_device');
    expect(erased).toEqual(['closed', 'owner-key', 'uploads', 'session', 'sdk']);
  },
);

function liveFixture(cleanup: () => Promise<void>) {
  const state = { released: 0, erased: 0, uploadsClosed: false, stopped: 0 };
  const key = new Uint8Array(32).fill(7);
  const erase = async () => {
    state.erased++;
    await cleanup();
  };
  const instance = Object.assign(Object.create(SuiteMatrix.prototype), {
    closed: false,
    network: { closed: false },
    clientModules: new Map(),
    temporaryKeys: new Map([['artificial', key]]),
    urls: new Set<string>(),
    release: () => {
      state.released++;
    },
    client: {
      stopClient: () => {
        state.stopped++;
      },
      clearStores: erase,
    },
    adapter: { forgetSession: erase },
    ownerKeys: { close: () => {}, destroy: erase },
    uploadCache: {
      close: () => {
        state.uploadsClosed = true;
      },
      destroy: erase,
    },
    onOfflineClear: () => {},
  }) as SuiteMatrix;
  return { instance, state, key };
}

test('forget holds the device lease until every credential store is erased, even if another cleanup closes it again', async () => {
  let release!: () => void, started!: () => void;
  const gate = new Promise<void>((done) => {
    release = done;
  });
  const seen = new Promise<void>((done) => {
    started = done;
  });
  const f = liveFixture(async () => {
    started();
    await gate;
  });
  const pending = f.instance.forget();
  await seen;
  try {
    f.instance.close();
    expect(f.state.released).toBe(0);
    expect(f.key.every((value) => value === 0)).toBe(true);
  } finally {
    release();
    await pending;
  }
  expect(f.state.released).toBe(1);
});

test('a stale closed client cannot erase stores after another client has acquired its device', async () => {
  const f = liveFixture(async () => {});
  f.instance.close();
  await expect(f.instance.forget()).rejects.toThrow('matrix_device');
  await f.instance.closeAndDrain();
  expect(f.state.erased).toBe(0);
  expect(f.state.released).toBe(1);
});

test('forget erases an unopened upload cache and concurrent requests share all cleanup', async () => {
  const f = liveFixture(async () => {});
  await Promise.all([f.instance.forget(), f.instance.forget()]);
  expect(f.state.erased).toBe(4);
  expect(f.state.uploadsClosed).toBe(true);
  expect(f.state.released).toBe(1);
});

test('forget drains cancelled storage jobs before erasing stores or releasing its device', async () => {
  const f = liveFixture(async () => {}),
    jobs = new StorageJobs();
  Object.assign(f.instance, { storageJobs: jobs });
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const pending = jobs.run('flush', async (signal) => {
    await gate;
    expect(signal.aborted).toBe(true);
    throw new Error('stopped');
  });
  const caught = Promise.allSettled([pending]);
  const forgetting = f.instance.forget();
  await new Promise((resolve) => setTimeout(resolve, 0));
  try {
    expect(f.state.erased).toBe(0);
    expect(f.state.released).toBe(0);
    expect(f.key.every((value) => value === 7)).toBe(true);
  } finally {
    release();
    await caught;
    await forgetting;
  }
  expect(f.key.every((value) => value === 0)).toBe(true);
  expect(f.state.erased).toBe(4);
  expect(f.state.released).toBe(1);
});

test('ordinary close retains the device lease until pending file cleanup finishes', async () => {
  const f = liveFixture(async () => {}),
    jobs = new StorageJobs();
  Object.assign(f.instance, { storageJobs: jobs });
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const pending = jobs.run('file', async () => {
    await gate;
    return 1;
  });
  const caught = Promise.allSettled([pending]);
  f.instance.close();
  try {
    expect(f.state.released).toBe(0);
    expect(f.state.erased).toBe(0);
  } finally {
    release();
    await caught;
    await jobs.close();
    await f.instance.closeAndDrain();
  }
  expect(f.state.released).toBe(1);
  expect(f.state.erased).toBe(0);
});

test('a recovery prompt cannot deadlock logout while a private storage job waits for its key', async () => {
  const f = liveFixture(async () => {}),
    jobs = new StorageJobs(),
    lifetime = new AbortController();
  let cancelled = false;
  const ui = recoveryUILifetime(
    {
      readRecoveryKey: () => new Promise(() => {}),
      saveRecoveryKey: async () => {},
      authenticate: async () => {},
      cancelPending: () => {
        cancelled = true;
      },
    },
    lifetime.signal,
  );
  Object.assign(f.instance, { storageJobs: jobs, network: { closed: false, lifetime }, ui });
  const pending = jobs.run('key', async () => ui.readRecoveryKey());
  const caught = Promise.allSettled([pending]);
  await f.instance.forget();
  await caught;
  expect(cancelled).toBe(true);
  expect(f.state.erased).toBe(4);
  expect(f.state.released).toBe(1);
});

test('closing uses AbortError for SDK network cancellation', async () => {
  const f = liveFixture(async () => {}),
    lifetime = new AbortController();
  Object.assign(f.instance, { network: { closed: false, lifetime } });
  f.instance.close();
  expect(lifetime.signal.aborted).toBe(true);
  expect(lifetime.signal.reason).toMatchObject({ name: 'AbortError' });
  await f.instance.closeAndDrain();
});

test.each([false, true])(
  'all loaded modules drain before SDK erasure and device release (module failure: %s)',
  async (failCore) => {
    const f = liveFixture(async () => {});
    let releaseCore!: () => void,
      releaseMail!: () => void,
      coreStarted!: () => void,
      mailStarted!: () => void;
    const coreGate = new Promise<void>((resolve) => {
      releaseCore = resolve;
    });
    const mailGate = new Promise<void>((resolve) => {
      releaseMail = resolve;
    });
    const coreSeen = new Promise<void>((resolve) => {
      coreStarted = resolve;
    });
    const mailSeen = new Promise<void>((resolve) => {
      mailStarted = resolve;
    });
    Object.assign(f.instance, {
      clientModules: new Map([
        [
          'core',
          {
            closeAndDrain: async () => {
              coreStarted();
              await coreGate;
              if (failCore) throw new Error('artificial core cleanup');
            },
          },
        ],
        [
          'mail',
          {
            closeAndDrain: async () => {
              mailStarted();
              await mailGate;
            },
          },
        ],
      ]),
    });
    f.instance.close();
    const closing = f.instance.closeAndDrain();
    const caught = Promise.allSettled([closing]);
    await Promise.all([coreSeen, mailSeen]);
    try {
      expect(f.state.released).toBe(0);
      expect(f.state.stopped).toBe(0);
      expect(f.key.every((value) => value === 7)).toBe(true);
      releaseCore();
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(f.state.released).toBe(0);
      expect(f.state.stopped).toBe(0);
      expect(f.key.every((value) => value === 7)).toBe(true);
    } finally {
      releaseCore();
      releaseMail();
    }
    const result = await caught;
    expect(result[0]?.status).toBe(failCore ? 'rejected' : 'fulfilled');
    expect(f.state.stopped).toBe(1);
    expect(f.state.released).toBe(1);
    expect(f.key.every((value) => value === 0)).toBe(true);
  },
);

test.each(['active', 'opening'])(
  'BFF %s owner IO drains before SDK erasure and lease release',
  async (phase) => {
    const f = liveFixture(async () => {});
    let release!: () => void, started!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const seen = new Promise<void>((resolve) => {
      started = resolve;
    });
    const owner = {
      closeAndDrain: async () => {
        started();
        await gate;
      },
    };
    Object.assign(
      f.instance,
      phase === 'active' ? { appSessionOwner: owner } : { openingAppOwner: Promise.resolve(owner) },
    );
    const closing = f.instance.closeAndDrain();
    await seen;
    await new Promise((resolve) => setTimeout(resolve, 0));
    try {
      expect(f.state.stopped).toBe(0);
      expect(f.state.released).toBe(0);
      expect(f.key.every((value) => value === 7)).toBe(true);
    } finally {
      release();
      await closing;
    }
    expect(f.state.stopped).toBe(1);
    expect(f.state.released).toBe(1);
    expect(f.key.every((value) => value === 0)).toBe(true);
  },
);

test('concurrent forget calls share the authenticated pending guard before any erasure', async () => {
  const f = liveFixture(async () => {});
  let guards = 0,
    release!: () => void,
    started!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const seen = new Promise<void>((resolve) => {
    started = resolve;
  });
  Object.assign(f.instance, {
    destructionGuard: async () => {
      guards++;
      started();
      await gate;
    },
  });
  const pending = Promise.allSettled([f.instance.forget(), f.instance.forget()]);
  await seen;
  try {
    expect(guards).toBe(1);
    expect(f.state.erased).toBe(0);
    expect(f.state.released).toBe(0);
  } finally {
    release();
  }
  expect((await pending).every((result) => result.status === 'fulfilled')).toBe(true);
  expect(f.state.erased).toBe(4);
  expect(f.state.released).toBe(1);
});

test('a denied pending guard preserves all stores and allows a later authenticated retry', async () => {
  const f = liveFixture(async () => {});
  let deny = true,
    guards = 0;
  Object.assign(f.instance, {
    destructionGuard: async () => {
      guards++;
      if (deny) throw new Error('pending');
    },
  });
  const denied = await Promise.allSettled([f.instance.forget(), f.instance.forget()]);
  expect(denied.every((result) => result.status === 'rejected')).toBe(true);
  expect(guards).toBe(1);
  expect(f.state.erased).toBe(0);
  expect(f.state.released).toBe(0);
  expect(f.key.every((value) => value === 7)).toBe(true);
  deny = false;
  await f.instance.forget();
  expect(guards).toBe(2);
  expect(f.state.erased).toBe(4);
  expect(f.state.released).toBe(1);
});
