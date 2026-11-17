/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { PqcResponseStreams } from './response-streams.js';

const reject = (): never => {
  throw new Error('rejected');
};

test('an aborted read and concurrent closes join the same pending cancellation', async () => {
  let entered!: () => void,
    finishCancel!: () => void,
    cancellations = 0,
    detached = 0;
  const reading = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const cancelGate = new Promise<void>((resolve) => {
    finishCancel = resolve;
  });
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    pull() {
      entered();
      return new Promise<void>(() => {});
    },
    cancel() {
      cancellations++;
      return cancelGate;
    },
  });
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(new Response(body), 10, new AbortController(), () => {
    detached++;
  });
  const abort = new AbortController();
  const read = streams.read(id, 0, 1, abort.signal).then(
    () => false,
    () => true,
  );
  await reading;
  abort.abort();
  let closed = false;
  const first = streams.closeStream(id).then(() => {
    closed = true;
  });
  const second = streams.closeStream(id);
  await Promise.resolve();
  await Promise.resolve();
  expect(cancellations).toBe(1);
  expect(detached).toBe(1);
  expect(closed).toBe(false);
  finishCancel();
  await Promise.all([first, second]);
  expect(await read).toBe(true);
  expect(body.locked).toBe(false);
  await streams.closeStream(id);
  expect(cancellations).toBe(1);
});

test('a nonce collision leaves the second response body unlocked and with its caller', async () => {
  let secondCancelled = 0;
  const streams = new PqcResponseStreams(reject, () => 'same');
  const first = new ReadableStream<Uint8Array<ArrayBuffer>>({ type: 'bytes' });
  const second = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    cancel() {
      secondCancelled++;
    },
  });
  streams.open(new Response(first), 1, new AbortController(), () => {});
  expect(() => streams.open(new Response(second), 1, new AbortController(), () => {})).toThrow();
  expect(second.locked).toBe(false);
  expect(secondCancelled).toBe(0);
  await second.cancel();
  expect(secondCancelled).toBe(1);
  await streams.close();
});

test('failure to acquire a byte reader does not acquire ownership of the body', async () => {
  const streams = new PqcResponseStreams(reject, () => 'id');
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>();
  expect(() => streams.open(new Response(body), 1, new AbortController(), () => {})).toThrow();
  expect(body.locked).toBe(false);
  await body.cancel();
  await streams.close();
});

test('an already locked body remains owned by its existing reader', async () => {
  let cancellations = 0;
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    cancel() {
      cancellations++;
    },
  });
  const response = new Response(body);
  const owner = body.getReader({ mode: 'byob' });
  const streams = new PqcResponseStreams(reject, () => 'id');
  expect(() => streams.open(response, 1, new AbortController(), () => {})).toThrow();
  await streams.close();
  expect(body.locked).toBe(true);
  expect(cancellations).toBe(0);
  await owner.cancel();
  owner.releaseLock();
  expect(cancellations).toBe(1);
});

test('a rejected cancellation is joined, unlocks the body and is reported', async () => {
  const failure = new Error('cancel failed');
  let calls = 0;
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    cancel() {
      calls++;
      return Promise.reject(failure);
    },
  });
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(new Response(body), 1, new AbortController(), () => {});
  const results = await Promise.allSettled([streams.closeStream(id), streams.closeStream(id)]);
  expect(results).toEqual([
    { status: 'rejected', reason: failure },
    { status: 'rejected', reason: failure },
  ]);
  expect(calls).toBe(1);
  expect(body.locked).toBe(false);
  await expect(streams.closeStream(id)).rejects.toBe(failure);
  await expect(streams.close()).rejects.toBeInstanceOf(AggregateError);
  await expect(streams.close()).rejects.toBeInstanceOf(AggregateError);
  expect(calls).toBe(1);
});

test('a release failure remains visible to every later close', async () => {
  const failure = new Error('release failed');
  let releases = 0;
  const response = {
    body: {
      getReader: () => ({
        cancel: async () => {},
        releaseLock() {
          releases++;
          throw failure;
        },
      }),
    },
  } as unknown as Response;
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(response, 1, new AbortController(), () => {});
  await expect(streams.closeStream(id)).rejects.toBe(failure);
  await expect(streams.closeStream(id)).rejects.toBe(failure);
  await expect(streams.close()).rejects.toBeInstanceOf(AggregateError);
  expect(releases).toBe(1);
});

test('synchronous abort reentry shares the close and its cancellation failure', async () => {
  const failure = new Error('cancel failed');
  let started!: () => void,
    failCancel!: (reason: Error) => void,
    cancellations = 0,
    detached = 0,
    reentered: Promise<void> | undefined;
  const entered = new Promise<void>((resolve) => {
    started = resolve;
  });
  const gate = new Promise<void>((_resolve, reject) => {
    failCancel = reject;
  });
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    cancel() {
      cancellations++;
      started();
      return gate;
    },
  });
  const streams = new PqcResponseStreams(reject, () => 'id');
  const controller = new AbortController();
  const id = streams.open(new Response(body), 1, controller, () => {
    detached++;
  });
  controller.signal.addEventListener('abort', () => {
    reentered = streams.closeStream(id);
    void reentered.catch(() => {});
  });
  const closing = streams.closeStream(id);
  await entered;
  expect(reentered).toBeDefined();
  expect(detached).toBe(1);
  expect(cancellations).toBe(1);
  failCancel(failure);
  const results = await Promise.allSettled([closing, reentered!]);
  expect(results).toEqual([
    { status: 'rejected', reason: failure },
    { status: 'rejected', reason: failure },
  ]);
  await expect(streams.closeStream(id)).rejects.toBe(failure);
  expect(body.locked).toBe(false);
  expect(detached).toBe(1);
  expect(cancellations).toBe(1);
});

test('a throwing detach still joins cancellation and keeps its failure sticky', async () => {
  const failure = new Error('detach failed');
  let started!: () => void,
    finish!: () => void,
    cancellations = 0,
    detachments = 0,
    closed = false;
  const entered = new Promise<void>((resolve) => {
    started = resolve;
  });
  const gate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    cancel() {
      cancellations++;
      started();
      return gate;
    },
  });
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(new Response(body), 1, new AbortController(), () => {
    detachments++;
    throw failure;
  });
  const closing = streams.closeStream(id).then(
    () => {
      closed = true;
      return null;
    },
    (error: unknown) => {
      closed = true;
      return error;
    },
  );
  await entered;
  expect(closed).toBe(false);
  expect(detachments).toBe(1);
  expect(cancellations).toBe(1);
  finish();
  expect(await closing).toBe(failure);
  await expect(streams.closeStream(id)).rejects.toBe(failure);
  expect(body.locked).toBe(false);
  expect(detachments).toBe(1);
  expect(cancellations).toBe(1);
});

test('close between the reader settling and read continuation rejects the bytes', async () => {
  let started!: () => void,
    finishRead!: (result: ReadableStreamReadResult<Uint8Array<ArrayBuffer>>) => void,
    cancelStarted!: () => void,
    finishCancel!: () => void,
    closing: Promise<void> | undefined,
    releases = 0,
    cancellations = 0,
    readFinished = false,
    closed = false;
  const entered = new Promise<void>((resolve) => {
    started = resolve;
  });
  const pending = new Promise<ReadableStreamReadResult<Uint8Array<ArrayBuffer>>>((resolve) => {
    finishRead = resolve;
  });
  const cancelling = new Promise<void>((resolve) => {
    cancelStarted = resolve;
  });
  const cancelGate = new Promise<void>((resolve) => {
    finishCancel = resolve;
  });
  const response = {
    body: {
      getReader: () => ({
        read() {
          started();
          return pending;
        },
        cancel() {
          cancellations++;
          cancelStarted();
          return cancelGate;
        },
        releaseLock() {
          releases++;
        },
      }),
    },
  } as unknown as Response;
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(response, 1, new AbortController(), () => {});
  const read = streams.read(id, 0, 1, new AbortController().signal).then(
    () => {
      readFinished = true;
      return null;
    },
    (error: unknown) => {
      readFinished = true;
      return error;
    },
  );
  await entered;
  // Resolving queues readStream's continuation; this close runs after it but
  // before the outer read continuation queued by its resolved promise.
  finishRead({ done: false, value: new Uint8Array([7]) });
  queueMicrotask(() => {
    closing = streams.closeStream(id).then(() => {
      closed = true;
    });
  });
  await cancelling;
  expect(closing).toBeDefined();
  expect(readFinished).toBe(false);
  expect(closed).toBe(false);
  expect(cancellations).toBe(1);
  finishCancel();
  await closing;
  expect(await read).toBeInstanceOf(Error);
  expect(releases).toBe(1);
  expect(cancellations).toBe(1);
});

test('detach and cancellation failures are both retained on later closes', async () => {
  const detachFailure = new Error('detach failed');
  const cancelFailure = new Error('cancel failed');
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    cancel: async () => {
      throw cancelFailure;
    },
  });
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(new Response(body), 1, new AbortController(), () => {
    throw detachFailure;
  });
  const failure = await streams.closeStream(id).then(
    () => null,
    (error: unknown) => error,
  );
  expect(failure).toBeInstanceOf(AggregateError);
  expect((failure as AggregateError).errors).toEqual([detachFailure, cancelFailure]);
  await expect(streams.closeStream(id)).rejects.toBe(failure);
  expect(body.locked).toBe(false);
});

test('caller abort after the inner read settles joins cancellation along with close', async () => {
  let readStarted!: () => void,
    finishRead!: (result: ReadableStreamReadResult<Uint8Array<ArrayBuffer>>) => void,
    cancelStarted!: () => void,
    finishCancel!: () => void,
    cancellations = 0,
    closed = false,
    readFinished = false;
  const entered = new Promise<void>((resolve) => {
    readStarted = resolve;
  });
  const pending = new Promise<ReadableStreamReadResult<Uint8Array<ArrayBuffer>>>((resolve) => {
    finishRead = resolve;
  });
  const cancelling = new Promise<void>((resolve) => {
    cancelStarted = resolve;
  });
  const cancelGate = new Promise<void>((resolve) => {
    finishCancel = resolve;
  });
  const response = {
    body: {
      getReader: () => ({
        read() {
          readStarted();
          return pending;
        },
        cancel() {
          cancellations++;
          cancelStarted();
          return cancelGate;
        },
        releaseLock() {},
      }),
    },
  } as unknown as Response;
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(response, 1, new AbortController(), () => {});
  const caller = new AbortController();
  const read = streams.read(id, 0, 1, caller.signal);
  const readResult = read.then(
    () => {
      readFinished = true;
      return null;
    },
    (error: unknown) => {
      readFinished = true;
      return error;
    },
  );
  await entered;
  finishRead({ done: false, value: new Uint8Array([7]) });
  queueMicrotask(() => caller.abort());
  await cancelling;
  const closing = streams.closeStream(id).then(() => {
    closed = true;
  });
  await Promise.resolve();
  expect(closed).toBe(false);
  expect(readFinished).toBe(false);
  expect(cancellations).toBe(1);
  finishCancel();
  await closing;
  expect(await readResult).toBe(caller.signal.reason);
  expect(cancellations).toBe(1);
});

test('owner abort cancels a pending read even when the caller signal stays open', async () => {
  const reason = new Error('owner revoked');
  let readStarted!: () => void,
    cancelStarted!: () => void,
    finishCancel!: () => void,
    cancellations = 0,
    cancelReason: unknown,
    closed = false,
    readFinished = false;
  const entered = new Promise<void>((resolve) => {
    readStarted = resolve;
  });
  const cancelling = new Promise<void>((resolve) => {
    cancelStarted = resolve;
  });
  const cancelGate = new Promise<void>((resolve) => {
    finishCancel = resolve;
  });
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    pull() {
      readStarted();
      return new Promise<void>(() => {});
    },
    cancel(value) {
      cancellations++;
      cancelReason = value;
      cancelStarted();
      return cancelGate;
    },
  });
  const streams = new PqcResponseStreams(reject, () => 'id');
  const owner = new AbortController();
  const caller = new AbortController();
  const id = streams.open(new Response(body), 1, owner, () => {});
  const read = streams.read(id, 0, 1, caller.signal).then(
    () => {
      readFinished = true;
      return false;
    },
    () => {
      readFinished = true;
      return true;
    },
  );
  await entered;
  owner.abort(reason);
  await cancelling;
  expect(caller.signal.aborted).toBe(false);
  expect(cancelReason).toBe(reason);
  const closing = streams.closeStream(id).then(() => {
    closed = true;
  });
  await Promise.resolve();
  expect(closed).toBe(false);
  expect(readFinished).toBe(false);
  expect(cancellations).toBe(1);
  finishCancel();
  await closing;
  expect(await read).toBe(true);
  expect(body.locked).toBe(false);
});

test('abort after inner read retains both abort and cancellation errors', async () => {
  const reason = new Error('caller aborted');
  const cancelFailure = new Error('cancel failed');
  let finishRead!: (result: ReadableStreamReadResult<Uint8Array<ArrayBuffer>>) => void;
  const pending = new Promise<ReadableStreamReadResult<Uint8Array<ArrayBuffer>>>((resolve) => {
    finishRead = resolve;
  });
  const response = {
    body: {
      getReader: () => ({
        read: () => pending,
        cancel: async () => {
          throw cancelFailure;
        },
        releaseLock() {},
      }),
    },
  } as unknown as Response;
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(response, 1, new AbortController(), () => {});
  const caller = new AbortController();
  const readResult = streams.read(id, 0, 1, caller.signal).then(
    () => null,
    (error: unknown) => error,
  );
  finishRead({ done: false, value: new Uint8Array([7]) });
  queueMicrotask(() => caller.abort(reason));
  const failure = await readResult;
  expect(failure).toBeInstanceOf(AggregateError);
  expect((failure as AggregateError).errors).toEqual([reason, cancelFailure]);
  await expect(streams.closeStream(id)).rejects.toBe(cancelFailure);
});

test('abort during a pending read retains cancellation failure without wrapping twice', async () => {
  const reason = new Error('owner revoked');
  const cancelFailure = new Error('cancel failed');
  let started!: () => void;
  const entered = new Promise<void>((resolve) => {
    started = resolve;
  });
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    type: 'bytes',
    pull() {
      started();
      return new Promise<void>(() => {});
    },
    cancel: async () => {
      throw cancelFailure;
    },
  });
  const owner = new AbortController();
  const streams = new PqcResponseStreams(reject, () => 'id');
  const id = streams.open(new Response(body), 1, owner, () => {});
  const readResult = streams.read(id, 0, 1, new AbortController().signal).then(
    () => null,
    (error: unknown) => error,
  );
  await entered;
  owner.abort(reason);
  const failure = await readResult;
  expect(failure).toBeInstanceOf(AggregateError);
  expect((failure as AggregateError).errors).toEqual([reason, cancelFailure]);
  await expect(streams.closeStream(id)).rejects.toBe(cancelFailure);
});
