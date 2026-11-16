/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { StorageJobs } from './storage-jobs.js';

test('shutdown cancels every job and waits for asynchronous cleanup before permitting reuse', async () => {
  const jobs = new StorageJobs();
  let release!: () => void,
    aborted = 0,
    cleaned = 0;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const run = (id: string) =>
    jobs.run(id, async (signal) => {
      await new Promise<void>((resolve) =>
        signal.addEventListener(
          'abort',
          () => {
            aborted++;
            resolve();
          },
          { once: true },
        ),
      );
      await gate;
      cleaned++;
      throw new Error('cancelled fixture');
    });
  const first = run('first'),
    second = run('second');
  const caught = Promise.allSettled([first, second]);
  let stopped = false;
  const stopping = jobs.close().then(() => {
    stopped = true;
  });
  expect(aborted).toBe(2);
  expect(stopped).toBe(false);
  expect(jobs.empty).toBe(false);
  await expect(jobs.run('new', async () => 0)).rejects.toThrow('matrix_device');
  release();
  await stopping;
  await caught;
  expect(cleaned).toBe(2);
  expect(jobs.empty).toBe(true);
  expect(stopped).toBe(true);
  await jobs.close();
});

test('duplicate jobs are rejected, inputs snapshot synchronously and parent cancellation cannot return late success', async () => {
  const jobs = new StorageJobs(),
    parent = new AbortController();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
      release = resolve;
    }),
    input = { version: 1 };
  let captured = 0;
  const pending = jobs.run(
    'same',
    async () => {
      captured = input.version;
      await gate;
      return captured;
    },
    parent.signal,
  );
  input.version = 2;
  expect(captured).toBe(1);
  await expect(jobs.run('same', async () => 99)).rejects.toThrow('matrix_locked');
  parent.abort(new Error('caller cancelled'));
  release();
  await expect(pending).rejects.toThrow('caller cancelled');
  expect(await jobs.run('same', async () => 3)).toBe(3);
  await jobs.close();
});

test('reentrant close and synchronous failure leave no orphaned job or late success', async () => {
  const jobs = new StorageJobs();
  let closing: Promise<void> | undefined;
  const pending = jobs.run('reentrant', () => {
    closing = jobs.close();
    return Promise.resolve('late');
  });
  await expect(pending).rejects.toThrow('matrix_device');
  await closing;
  expect(jobs.empty).toBe(true);
  const second = new StorageJobs();
  await expect(
    second.run('throws', () => {
      throw new Error('sync failure');
    }),
  ).rejects.toThrow('sync failure');
  expect(second.empty).toBe(true);
  await second.close();
});

test('catalog queue survives a failed writer and shutdown drains queued work without starting it', async () => {
  const jobs = new StorageJobs();
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const order: string[] = [];
  const first = jobs.exclusive('catalog', async () => {
    order.push('first');
    await gate;
    throw new Error('failed writer');
  });
  const second = jobs.exclusive('catalog', () => {
    order.push('second');
    return Promise.resolve(2);
  });
  const caught = Promise.allSettled([first, second]);
  await Promise.resolve();
  expect(order).toEqual(['first']);
  release();
  expect(await caught).toEqual([
    { status: 'rejected', reason: expect.any(Error) },
    { status: 'fulfilled', value: 2 },
  ]);
  expect(order).toEqual(['first', 'second']);
  let finish!: () => void;
  const cleanup = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const active = jobs.exclusive('catalog', async (signal) => {
    await new Promise<void>((resolve) =>
      signal.addEventListener('abort', () => resolve(), { once: true }),
    );
    await cleanup;
  });
  const waiting = jobs.exclusive('catalog', () => {
    order.push('unexpected');
    return Promise.resolve();
  });
  const settled = Promise.allSettled([active, waiting]);
  await Promise.resolve();
  const closing = jobs.close();
  expect(jobs.empty).toBe(false);
  finish();
  await closing;
  expect((await settled).every((r) => r.status === 'rejected')).toBe(true);
  expect(order).toEqual(['first', 'second']);
  expect(jobs.empty).toBe(true);
});

test('retained SDK jobs survive cancellation until actual cleanup releases them', async () => {
  const jobs = new StorageJobs(),
    controller = new AbortController();
  const release = await jobs.retain(controller.signal);
  let drained = false;
  const closing = jobs.close().then(() => {
    drained = true;
  });
  controller.abort();
  await Promise.resolve();
  expect(drained).toBe(false);
  expect(jobs.empty).toBe(false);
  await release();
  await release();
  await closing;
  expect(drained).toBe(true);
  expect(jobs.empty).toBe(true);
});
test('a pre-aborted retained SDK lease never registers work', async () => {
  const jobs = new StorageJobs(),
    controller = new AbortController();
  controller.abort();
  await expect(jobs.retain(controller.signal)).rejects.toThrow();
  expect(jobs.empty).toBe(true);
});
