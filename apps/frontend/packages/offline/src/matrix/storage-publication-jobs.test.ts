/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { MatrixEvent } from 'matrix-js-sdk';
import {
  StoragePublicationJobs,
  type StoragePublicationLease,
} from './storage-publication-jobs.js';
const target = { room_id: '!private:test', device_id: 'DEVICE' };
test('event jobs retain original policy for concurrent events and reject cached retries after completion', async () => {
  const jobs = new StoragePublicationJobs(() => {}),
    first = new AbortController(),
    second = new AbortController();
  let releaseFirst!: () => void, releaseSecond!: () => void;
  const heldFirst = new Promise<void>((resolve) => {
      releaseFirst = resolve;
    }),
    heldSecond = new Promise<void>((resolve) => {
      releaseSecond = resolve;
    });
  const a = new MatrixEvent({ room_id: target.room_id, type: 'm.room.encrypted', content: {} }),
    b = new MatrixEvent({ room_id: target.room_id, type: 'm.room.encrypted', content: {} });
  a.setTxnId('first');
  b.setTxnId('second');
  const called: string[] = [];
  const policy = (name: string, signal: AbortSignal) => ({
    target,
    signal,
    assertOpen: () => signal.throwIfAborted(),
    authorize: async () => {
      called.push(name);
    },
  });
  const pa = jobs.run(target, 'first', first.signal, policy('first', first.signal), async () => {
    await jobs.authorize(a);
    await heldFirst;
    return 1;
  });
  const pb = jobs.run(
    target,
    'second',
    second.signal,
    policy('second', second.signal),
    async () => {
      await jobs.authorize(b);
      await heldSecond;
      return 2;
    },
  );
  await Promise.resolve();
  await Promise.resolve();
  expect(called).toEqual(['first', 'second']);
  releaseFirst();
  await pa;
  await expect(jobs.authorize(a)).rejects.toThrow();
  await jobs.authorize(b);
  expect(called).toEqual(['first', 'second', 'second']);
  releaseSecond();
  await pb;
  await expect(jobs.authorize(b)).rejects.toThrow();
});
test('same SDK transaction cannot be rebound while its first operation owns it', async () => {
  const jobs = new StoragePublicationJobs(() => {}),
    life = new AbortController();
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  const pending = jobs.run(target, 'immutable', life.signal, undefined, async () => {
    await held;
    return 1;
  });
  await expect(
    jobs.run(target, 'immutable', life.signal, undefined, async () => 2),
  ).rejects.toThrow('locked');
  release();
  await pending;
});

test('admission cache leases belong to one live original SDK entry and release with it', async () => {
  const jobs = new StoragePublicationJobs(() => {}),
    life = new AbortController();
  const event = new MatrixEvent({ room_id: target.room_id, type: 'm.room.encrypted', content: {} });
  event.setTxnId('original');
  expect(() => jobs.lease(event)).toThrow();
  let captured: StoragePublicationLease | undefined,
    released = 0;
  await jobs.run(target, 'original', life.signal, undefined, async () => {
    await jobs.authorize(event);
    captured = jobs.lease(event);
    captured.onRelease(() => {
      released++;
    });
    captured.assertOpen();
    expect(captured.target).toEqual(target);
  });
  expect(released).toBe(1);
  expect(() => captured!.assertOpen()).toThrow();
  await jobs.run(target, 'original', life.signal, undefined, async () => {
    await expect(jobs.authorize(event)).rejects.toThrow();
    const next = new MatrixEvent({
      room_id: target.room_id,
      type: 'm.room.encrypted',
      content: {},
    });
    next.setTxnId('original');
    await jobs.authorize(next);
    expect(jobs.lease(next).token).not.toBe(captured!.token);
  });
});
