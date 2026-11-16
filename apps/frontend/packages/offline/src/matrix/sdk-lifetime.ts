/* SPDX-License-Identifier: Apache-2.0 */
/** Mechanical lifetime coupling, never an SDK identity or authorization grant. */
export interface SDKLifetimeMember {
  retire: () => void;
  /** Must settle only after actual IO joins, even when reporting failure. */
  drainIO: () => Promise<void>;
  erase: () => void | Promise<void>;
}
export class SDKLifetimePair {
  private done: Promise<void> | undefined;
  get retired(): boolean {
    return this.done !== undefined;
  }
  constructor(private readonly members: readonly [SDKLifetimeMember, SDKLifetimeMember]) {}
  retire(): Promise<void> {
    if (this.done) return this.done;
    let resolve!: () => void, reject!: (error: unknown) => void;
    this.done = new Promise<void>((accept, decline) => {
      resolve = accept;
      reject = decline;
    });
    // Install the barrier before calling either owner: close() can reenter.
    const failures: unknown[] = [];
    for (const member of this.members) {
      try {
        member.retire();
      } catch (error) {
        failures.push(error);
      }
    }
    void (async () => {
      const joined = await Promise.allSettled(
        this.members.map((member) => Promise.resolve().then(() => member.drainIO())),
      );
      for (const result of joined) if (result.status === 'rejected') failures.push(result.reason);
      // Every drain promise has settled after joining real IO. Attempt every
      // erasure even if a different cleanup fails; never report a false ACK.
      const erased = await Promise.allSettled(
        this.members.map((member) => Promise.resolve().then(() => member.erase())),
      );
      for (const result of erased) if (result.status === 'rejected') failures.push(result.reason);
      if (failures.length) throw new AggregateError(failures, 'matrix_device');
    })().then(resolve, reject);
    void this.done.catch(() => {});
    return this.done;
  }
}
