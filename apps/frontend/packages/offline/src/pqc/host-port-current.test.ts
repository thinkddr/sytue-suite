// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test';
import { PqcHostPorts, type HostCall, type HostScope } from './host-port.js';
const scope = Object.freeze({
  tenant: 'tenant',
  subject: 'owner',
  product: 'mail',
});
function fixture() {
  const lease = new AbortController();
  let runtime = true,
    authority = true,
    runs = 0,
    guards = 0,
    id = 0;
  const binding = {
    scope: { ...scope, subject: String(scope.subject) },
    matrixUserID: '@owner:test',
    deviceID: 'DEVICE',
  };
  const ports = new PqcHostPorts(
    () => {
      if (!runtime) throw Error('runtime');
    },
    () => {},
    () => binding,
  );
  const assertOpen = () => {
    if (lease.signal.aborted) throw Error('lease');
  };
  const port = ports.register({
    scope,
    lease: { signal: lease.signal, assertOpen },
    run: async (action, parent) => {
      runs++;
      if (!authority) throw Error('pre');
      guards++;
      const out = await action(parent!);
      try {
        if (!authority) throw Error('post');
        guards++;
        return out;
      } catch (error) {
        if (out instanceof Uint8Array) out.fill(0);
        throw error;
      }
    },
    handlers: {
      'authority.revalidate': async () => new Uint8Array(),
      'storage.record.read': async () => new Uint8Array([7]),
    },
    cleanup: async () => {},
  });
  const call = (
    method: HostCall['method'],
    requested: HostScope | null = scope,
    worker = () => {},
  ) =>
    ports.dispatch(
      {
        type: 'host.call',
        id: ++id,
        port: port.id,
        scope: requested,
        method,
        payload: new Uint8Array(),
      },
      worker,
    );
  return {
    ports,
    port,
    binding,
    call,
    lease,
    setRuntime: (value: boolean) => {
      runtime = value;
    },
    setAuthority: (value: boolean) => {
      authority = value;
    },
    counts: () => ({ runs, guards }),
  };
}
describe('local pinned current descriptor', () => {
  test('ten descriptors never enter fresh authority; sensitive IO keeps both guards', async () => {
    const f = fixture();
    for (let i = 0; i < 10; i++) {
      const raw = await f.call('current', null);
      const identity = JSON.parse(new TextDecoder().decode(raw));
      raw.fill(0);
      expect(identity).toEqual({
        v: 1,
        scope,
        binding: { matrix_user_id: '@owner:test', device_id: 'DEVICE' },
      });
    }
    expect(f.counts()).toEqual({ runs: 0, guards: 0 });
    expect([...(await f.call('storage.record.read'))]).toEqual([7]);
    expect(f.counts()).toEqual({ runs: 1, guards: 2 });
    f.setAuthority(false);
    expect(await f.call('current')).toBeInstanceOf(Uint8Array);
    await expect(f.call('storage.record.read')).rejects.toThrow();
    f.port.revoke();
    await f.port.drain();
  });
  test('descriptor copies the binding and retains scope, worker, runtime and lease fences', async () => {
    const f = fixture();
    f.binding.deviceID = 'MUTATED';
    f.binding.scope.subject = 'mutated';
    const current = JSON.parse(new TextDecoder().decode(await f.call('current')));
    expect(current.binding.device_id).toBe('DEVICE');
    expect(current.scope.subject).toBe('owner');
    await expect(f.call('current', { ...scope, subject: 'foreign' })).rejects.toThrow();
    await expect(
      f.call('current', scope, () => {
        throw Error('worker');
      }),
    ).rejects.toThrow();
    f.setRuntime(false);
    await expect(f.call('current')).rejects.toThrow();
    f.setRuntime(true);
    f.lease.abort();
    await expect(f.call('current')).rejects.toThrow();
    await f.port.drain();
  });
  test('sensitive post rejection erases the produced buffer', async () => {
    const lease = new AbortController();
    let allow = true;
    const owned = new Uint8Array([1, 2, 3]);
    const ports = new PqcHostPorts(
      () => {},
      () => {},
      () => ({ scope, matrixUserID: '@owner:test', deviceID: 'DEVICE' }),
    );
    const port = ports.register({
      scope,
      lease: { signal: lease.signal, assertOpen: () => {} },
      run: async (action, parent) => {
        if (!allow) throw Error('pre');
        const value = await action(parent!);
        if (!allow) {
          if (value instanceof Uint8Array) value.fill(0);
          throw Error('post');
        }
        return value;
      },
      handlers: {
        'storage.record.read': async () => {
          allow = false;
          return owned;
        },
      },
      cleanup: async () => {},
    });
    await expect(
      ports.dispatch(
        {
          type: 'host.call',
          id: 1,
          port: port.id,
          scope,
          method: 'storage.record.read',
          payload: new Uint8Array(),
        },
        () => {},
      ),
    ).rejects.toThrow();
    expect([...owned]).toEqual([0, 0, 0]);
    port.revoke();
    await port.drain();
  });
  test('descriptor checks the final worker generation and runtime, and denies replay', async () => {
    const f = fixture();
    let workers = 0;
    await expect(
      f.call('current', scope, () => {
        if (++workers === 2) throw Error('late worker');
      }),
    ).rejects.toThrow();
    workers = 0;
    await expect(
      f.call('current', scope, () => {
        if (++workers === 2) f.setRuntime(false);
      }),
    ).rejects.toThrow();
    f.setRuntime(true);
    await expect(
      f.ports.dispatch(
        {
          type: 'host.call',
          id: 1,
          port: f.port.id,
          scope,
          method: 'current',
          payload: new Uint8Array(),
        },
        () => {},
      ),
    ).rejects.toThrow();
    f.port.revoke();
    await f.port.drain();
  });
  test('pins the checked method before an asynchronous owner runner', async () => {
    const lease = new AbortController();
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let writes = 0;
    const ports = new PqcHostPorts(
      () => {},
      () => {},
      () => ({ scope, matrixUserID: '@owner:test', deviceID: 'DEVICE' }),
    );
    const port = ports.register({
      scope,
      lease: { signal: lease.signal, assertOpen: () => {} },
      run: async (action, signal) => {
        await gate;
        return action(signal);
      },
      handlers: {
        'storage.record.read': async () => new Uint8Array([1]),
        'storage.record.put': async () => {
          writes++;
          return new Uint8Array([2]);
        },
      },
      cleanup: async () => {},
    });
    const message: HostCall = {
      type: 'host.call',
      id: 1,
      port: port.id,
      scope,
      method: 'storage.record.read',
      payload: new Uint8Array(),
    };
    const pending = ports.dispatch(message, () => {});
    message.method = 'storage.record.put';
    release();
    expect([...(await pending)]).toEqual([1]);
    expect(writes).toBe(0);
    port.revoke();
    await port.drain();
  });
});
