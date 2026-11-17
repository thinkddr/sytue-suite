/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { PqcPortGroup } from './port-lifetime.js';
import type { HostPortHandle } from './host-port.js';
function held() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}
test('revoking either lifetime closes all port grants before Go ACK and retains cleanup until actual drain', async () => {
  for (const index of [0, 1]) {
    const owners = [new AbortController(), new AbortController()];
    const group = new PqcPortGroup(owners.map((owner) => owner.signal));
    const gate = held(),
      revoked: string[] = [];
    for (const id of ['canonical', 'legacy', 'migration-writer'])
      group.add({
        id,
        scope: { tenant: 'acme', subject: 'owner', product: 'mail' },
        revoke: () => {
          revoked.push(id);
        },
        drain: () => gate.promise,
      });
    owners[index]!.abort();
    expect(group.signal.aborted).toBe(true);
    expect(revoked).toEqual(['canonical', 'legacy', 'migration-writer']);
    expect(() => group.assertOpen()).toThrow();
    let drained = false;
    const closing = group.closeAndDrain().then(() => {
      drained = true;
    });
    await Promise.resolve();
    expect(drained).toBe(false);
    gate.release();
    await closing;
    expect(drained).toBe(true);
    expect(group.closeAndDrain()).toBe(group.closeAndDrain());
  }
});
test('failed cleanup still drains all grants and never reports a successful group ACK', async () => {
  const group = new PqcPortGroup([]),
    gate = held();
  let completed = false;
  group.add({
    id: 'a',
    scope: { tenant: 't', subject: 's', product: 'mail' },
    revoke() {},
    drain: async () => {
      throw Error('fixture cleanup failure');
    },
  } as HostPortHandle);
  group.add({
    id: 'b',
    scope: { tenant: 't', subject: 's', product: 'mail' },
    revoke() {},
    drain: async () => {
      await gate.promise;
      completed = true;
    },
  } as HostPortHandle);
  const closing = group.closeAndDrain();
  await Promise.resolve();
  expect(completed).toBe(false);
  gate.release();
  await expect(closing).rejects.toThrow('host_port_group_drain');
  expect(completed).toBe(true);
});
