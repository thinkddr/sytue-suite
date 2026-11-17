/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import {
  PqcHostPorts,
  type ResolvedHostDelegation,
  type HostBinding,
  type HostPortOptions,
} from './host-port.js';
const scope = { tenant: 'artificial-delegation', subject: 'owner', product: 'mail' };
const current: HostBinding = {
  scope,
  matrixUserID: '@isolated:matrix.test',
  deviceID: 'NEW_DEVICE',
};
function options(): HostPortOptions {
  const controller = new AbortController();
  return {
    scope,
    lease: { signal: controller.signal, assertOpen: () => {} },
    run: async (action, signal) => action(signal),
    handlers: {},
    cleanup: async () => {},
  };
}
test('closed resolver rejects forged tokens and exposes the actual delegated SDK binding', async () => {
  const grants = new WeakMap<object, ResolvedHostDelegation>();
  const token = Object.freeze(Object.create(null));
  const legacy: HostBinding = {
    scope,
    matrixUserID: '@canonical:matrix.test',
    deviceID: 'OLD_DEVICE',
  };
  grants.set(token, { binding: legacy, options: options() });
  const ports = new PqcHostPorts(
    () => {},
    () => {},
    () => current,
    (cap) => {
      const grant = grants.get(cap);
      if (!grant) throw Error('unissued');
      return grant;
    },
  );
  expect(() => ports.registerDelegated({ binding: legacy, options: options() })).toThrow();
  const port = ports.registerDelegated(token);
  const bytes = await ports.dispatch(
    {
      type: 'host.call',
      id: 1,
      port: port.id,
      scope: null,
      method: 'current',
      payload: new Uint8Array(),
    },
    () => {},
  );
  const identity = JSON.parse(new TextDecoder().decode(bytes));
  bytes.fill(0);
  expect(identity.scope).toEqual(scope);
  expect(identity.binding).toEqual({
    matrix_user_id: legacy.matrixUserID,
    device_id: legacy.deviceID,
  });
  port.revoke();
  await port.drain();
  await expect(
    ports.dispatch(
      {
        type: 'host.call',
        id: 2,
        port: port.id,
        scope: null,
        method: 'current',
        payload: new Uint8Array(),
      },
      () => {},
    ),
  ).rejects.toThrow();
});
test('even a resolved grant cannot cross the owner tenant actor or product', () => {
  for (const other of [
    { ...scope, tenant: 'other' },
    { ...scope, subject: 'other' },
    { ...scope, product: 'calendar' },
  ]) {
    const ports = new PqcHostPorts(
      () => {},
      () => {},
      () => current,
      () => ({ binding: { ...current, scope: other }, options: { ...options(), scope: other } }),
    );
    expect(() => ports.registerDelegated({})).toThrow();
  }
});
