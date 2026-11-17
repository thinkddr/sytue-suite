/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { recoveryUILifetime } from './recovery-ui.js';
test('closing a device rejects every recovery prompt immediately and ignores late host results', async () => {
  const controller = new AbortController();
  let late!: (value: string) => void,
    cancelled = 0;
  const ui = recoveryUILifetime(
    {
      readRecoveryKey: () =>
        new Promise((resolve) => {
          late = resolve;
        }),
      saveRecoveryKey: () => new Promise(() => {}),
      authenticate: () => new Promise(() => {}),
      cancelPending: () => {
        cancelled++;
      },
    },
    controller.signal,
  );
  const calls = [
    ui.readRecoveryKey(),
    ui.saveRecoveryKey('artificial'),
    ui.authenticate('https://matrix.test'),
  ];
  const outcomes = Promise.allSettled(calls);
  controller.abort(new Error('device closed'));
  ui.cancelPending?.();
  expect((await outcomes).every((result) => result.status === 'rejected')).toBe(true);
  expect(cancelled).toBe(1);
  late('late artificial key');
  await expect(ui.readRecoveryKey()).rejects.toThrow('device closed');
});

test('concurrent storage reads share one recovery prompt and a later operation can prompt again', async () => {
  const controller = new AbortController();
  let calls = 0,
    release!: (key: string) => void;
  const ui = recoveryUILifetime(
    {
      readRecoveryKey: () => {
        calls++;
        return new Promise((resolve) => {
          release = resolve;
        });
      },
      saveRecoveryKey: async () => {},
      authenticate: async () => {},
    },
    controller.signal,
  );
  const first = ui.readRecoveryKey(),
    second = ui.readRecoveryKey();
  expect(calls).toBe(1);
  release('artificial recovery input');
  expect(await Promise.all([first, second])).toEqual([
    'artificial recovery input',
    'artificial recovery input',
  ]);
  const next = ui.readRecoveryKey();
  expect(calls).toBe(2);
  controller.abort(new Error('closed'));
  await expect(next).rejects.toThrow('closed');
});
