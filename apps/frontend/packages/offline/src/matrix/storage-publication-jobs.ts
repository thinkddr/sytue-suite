/* SPDX-License-Identifier: Apache-2.0 */
import type { MatrixEvent } from 'matrix-js-sdk';
import { MatrixSecurityError } from './types.js';
import type { StoragePublicationTarget } from './storage.js';
import type { StoragePublicationPolicy } from './storage-repository.js';

export interface StoragePublicationLease {
  readonly token: object;
  readonly target: Readonly<StoragePublicationTarget>;
  readonly transaction: string;
  readonly signal: AbortSignal;
  assertOpen: () => void;
  onRelease: (cleanup: () => void) => void;
}
interface Entry {
  readonly token: object;
  readonly cleanup: Set<() => void>;
  readonly target: Readonly<StoragePublicationTarget>;
  readonly transaction: string;
  readonly signal: AbortSignal;
  policy: StoragePublicationPolicy | undefined;
  closed: boolean;
}
/** One owner SDK, immutable per-event operations. Cached scheduler retries keep
 * their original closed entry through a WeakMap; they never adopt a later job. */
export class StoragePublicationJobs {
  private readonly active = new Map<string, Entry>();
  private readonly events = new WeakMap<MatrixEvent, Entry>();
  constructor(private readonly assertOpen: () => void) {}
  private key(room: string, transaction: string): string {
    return room + '\0' + transaction;
  }
  private check(entry: Entry): void {
    this.assertOpen();
    entry.signal.throwIfAborted();
    if (entry.closed) throw new MatrixSecurityError('device');
    entry.policy?.assertOpen();
  }
  async run<T>(
    target: StoragePublicationTarget,
    transaction: string,
    signal: AbortSignal,
    policy: StoragePublicationPolicy | undefined,
    action: () => Promise<T>,
  ): Promise<T> {
    this.assertOpen();
    signal.throwIfAborted();
    const key = this.key(target.room_id, transaction);
    if (this.active.has(key)) throw new MatrixSecurityError('locked');
    const entry: Entry = {
      token: Object.freeze(Object.create(null) as object),
      cleanup: new Set(),
      target: Object.freeze({ ...target }),
      transaction,
      signal,
      policy,
      closed: false,
    };
    this.active.set(key, entry);
    try {
      this.check(entry);
      const result = await action();
      this.check(entry);
      return result;
    } finally {
      entry.closed = true;
      for (const cleanup of entry.cleanup) {
        try {
          cleanup();
        } catch {
          /* Remaining retained public descriptors still release. */
        }
      }
      entry.cleanup.clear();
      entry.policy = undefined;
      if (this.active.get(key) === entry) this.active.delete(key);
    }
  }
  lease(event: MatrixEvent): Readonly<StoragePublicationLease> {
    const entry = this.events.get(event);
    if (!entry) throw new MatrixSecurityError('unverified');
    this.check(entry);
    return Object.freeze({
      token: entry.token,
      target: entry.target,
      transaction: entry.transaction,
      signal: entry.signal,
      assertOpen: () => this.check(entry),
      onRelease: (cleanup: () => void) => {
        this.check(entry);
        entry.cleanup.add(cleanup);
      },
    });
  }
  async authorize(event: MatrixEvent): Promise<AbortSignal | undefined> {
    this.assertOpen();
    const room = event.getRoomId(),
      transaction = event.getTxnId();
    if (!room || !transaction) throw new MatrixSecurityError('reference');
    let entry = this.events.get(event);
    if (!entry) {
      entry = this.active.get(this.key(room, transaction));
      if (entry) this.events.set(event, entry);
    }
    if (!entry) return undefined;
    this.check(entry);
    if (entry.target.room_id !== room || entry.transaction !== transaction)
      throw new MatrixSecurityError('reference');
    await entry.policy?.authorize(transaction, entry.target, entry.signal);
    this.check(entry);
    return entry.signal;
  }
}
