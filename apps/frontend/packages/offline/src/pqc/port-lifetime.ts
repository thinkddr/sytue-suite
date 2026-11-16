/* SPDX-License-Identifier: Apache-2.0 */
import type { HostLease, HostPortHandle } from './host-port.js';
/** Mechanical lifetime only: no binding, account metadata or authority is minted.
 * Every member becomes unusable synchronously before any worker ACK can arrive. */
export class PqcPortGroup implements HostLease {
  private readonly controller = new AbortController();
  readonly signal = this.controller.signal;
  private closed = false;
  private readonly handles = new Set<HostPortHandle>();
  private readonly detach: Array<() => void> = [];
  private readonly failures: unknown[] = [];
  private draining: Promise<void> | undefined;
  constructor(signals: readonly AbortSignal[]) {
    const retire = () => this.revoke();
    for (const signal of signals) {
      if (signal.aborted) {
        this.revoke();
        break;
      }
      signal.addEventListener('abort', retire, { once: true });
      this.detach.push(() => signal.removeEventListener('abort', retire));
    }
  }
  assertOpen(): void {
    this.signal.throwIfAborted();
  }
  add(handle: HostPortHandle): HostPortHandle {
    this.assertOpen();
    this.handles.add(handle);
    return handle;
  }
  revoke(): void {
    if (this.closed) return;
    this.closed = true;
    this.controller.abort(new DOMException('Aborted', 'AbortError'));
    for (const detach of this.detach) detach();
    this.detach.length = 0;
    for (const handle of this.handles) {
      try {
        handle.revoke();
      } catch (error) {
        this.failures.push(error);
      }
    }
  }
  closeAndDrain(): Promise<void> {
    if (this.draining) return this.draining;
    let finish!: () => void, fail!: (error: unknown) => void;
    this.draining = new Promise<void>((resolve, reject) => {
      finish = resolve;
      fail = reject;
    });
    void this.draining.catch(() => {});
    this.revoke();
    void Promise.allSettled(
      [...this.handles].map((handle) => Promise.resolve().then(() => handle.drain())),
    )
      .then((results) => {
        for (const result of results)
          if (result.status === 'rejected') this.failures.push(result.reason);
        this.handles.clear();
        if (this.failures.length) throw new AggregateError(this.failures, 'host_port_group_drain');
      })
      .then(finish, fail);
    return this.draining;
  }
}
