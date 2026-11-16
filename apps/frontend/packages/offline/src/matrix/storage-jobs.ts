/* SPDX-License-Identifier: Apache-2.0 */
import { MatrixSecurityError, matrixFailure } from './types.js';

export type StorageJobRunner = <T>(
  name: string,
  action: (signal: AbortSignal) => Promise<T>,
  parent?: AbortSignal,
) => Promise<T>;

/** Shared entry guard for repositories and file streams; snapshots run synchronously. */
export function storageWork<T>(
  runner: StorageJobRunner | undefined,
  assertOpen: () => void,
  parent: AbortSignal | undefined,
  action: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  parent?.throwIfAborted();
  assertOpen();
  const guarded = async (signal: AbortSignal): Promise<T> => {
    signal.throwIfAborted();
    assertOpen();
    const value = await action(signal);
    signal.throwIfAborted();
    assertOpen();
    return value;
  };
  return runner
    ? runner(crypto.randomUUID(), guarded, parent)
    : guarded(parent ?? new AbortController().signal);
}

/** One leased device owns these operations. Stop first, then drain before cache/key erasure. */
export class StorageJobs {
  private closed = false;
  private closing: Promise<void> | undefined;
  private readonly queues = new Map<string, Promise<void>>();
  private readonly jobs = new Map<
    string,
    { controller: AbortController; done: Promise<unknown> }
  >();
  get empty(): boolean {
    return this.jobs.size === 0;
  }
  run<T>(
    name: string,
    action: (signal: AbortSignal) => Promise<T>,
    parent?: AbortSignal,
  ): Promise<T> {
    if (this.closed) return Promise.reject(new MatrixSecurityError('device'));
    if (!name || this.jobs.has(name)) return Promise.reject(new MatrixSecurityError('locked'));
    if (parent?.aborted) return Promise.reject(matrixFailure(parent.reason));
    const controller = new AbortController();
    const abort = () => controller.abort(matrixFailure(parent?.reason));
    parent?.addEventListener('abort', abort, { once: true });
    let resolve!: (value: T) => void, reject!: (reason: unknown) => void;
    const done = new Promise<T>((accept, decline) => {
      resolve = accept;
      reject = decline;
    });
    // Register before invoking user code: a synchronous/reentrant close must see
    // this job, while input snapshots still happen before this method returns.
    this.jobs.set(name, { controller, done });
    const finish = () => {
      parent?.removeEventListener('abort', abort);
      this.jobs.delete(name);
    };
    try {
      void Promise.resolve(action(controller.signal)).then(
        (value) => {
          finish();
          if (controller.signal.aborted) reject(matrixFailure(controller.signal.reason));
          else resolve(value);
        },
        (error) => {
          finish();
          reject(matrixFailure(error));
        },
      );
    } catch (error) {
      finish();
      reject(matrixFailure(error));
    }
    return done;
  }
  /** Retain the actual SDK job across several worker RPCs. Cancellation stops
   * new work but cannot release the owner before the Go/host cleanup joins. */
  async retain(parent?: AbortSignal): Promise<() => Promise<void>> {
    let enter!: () => void, reject!: (error: unknown) => void, release!: () => void;
    const ready = new Promise<void>((resolve, decline) => {
      enter = resolve;
      reject = decline;
    });
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const work = this.run(
      crypto.randomUUID(),
      async (signal) => {
        signal.throwIfAborted();
        enter();
        await held;
      },
      parent,
    );
    void work.catch(reject);
    await ready;
    let closing: Promise<void> | undefined;
    return () => {
      if (!closing) {
        release();
        closing = work.then(
          () => undefined,
          () => undefined,
        );
      }
      return closing;
    };
  }
  /** Serialize catalog workflows on the SDK's existing exclusive device lease. */
  exclusive<T>(
    key: string,
    action: (signal: AbortSignal) => Promise<T>,
    parent?: AbortSignal,
  ): Promise<T> {
    const previous = this.queues.get(key) ?? Promise.resolve();
    const work = this.run(
      crypto.randomUUID(),
      async (signal) => {
        await previous;
        signal.throwIfAborted();
        return action(signal);
      },
      parent,
    );
    const done = work.then(
      () => {},
      () => {},
    );
    this.queues.set(key, done);
    void done.then(() => {
      if (this.queues.get(key) === done) this.queues.delete(key);
    });
    return work;
  }
  close(): Promise<void> {
    if (this.closing) return this.closing;
    this.closed = true;
    const entries = [...this.jobs.values()];
    // Install before abort callbacks can reenter close(). Rejected work must not
    // prevent the remaining jobs or credential stores from being cleaned up.
    this.closing = Promise.allSettled(entries.map((entry) => entry.done)).then(() => undefined);
    for (const entry of entries) entry.controller.abort(new MatrixSecurityError('device'));
    return this.closing;
  }
}
