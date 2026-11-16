/* SPDX-License-Identifier: Apache-2.0 */
import { decodeHostObject, encodeHostJSON } from './host-codec.js';
import { newHostPortID } from './host-port.js';
import type {
  HostCallContext,
  HostHandler,
  HostLease,
  HostMethod,
  HostScope,
} from './host-port.js';

/** Created by SuiteMatrix from its real retained SDK jobs and authenticated
 * application-session witness. Consumers cannot choose a different binding. */
export interface PrivateWorkWitness {
  revalidate: (signal: AbortSignal) => Promise<void>;
  acquire: (signal: AbortSignal) => Promise<() => void | Promise<void>>;
}
/** Actual work leases outlive an individual nested RPC. Port cleanup runs only
 * after Go cancel/drain acknowledgement and releases every remaining witness. */
export class PqcWorkPort {
  private closed = false;
  private readonly releases = new Map<string, () => void | Promise<void>>();
  readonly handlers: Partial<Record<Exclude<HostMethod, 'current'>, HostHandler>>;
  constructor(scope: Readonly<HostScope>, lease: HostLease, witness: PrivateWorkWitness) {
    const bound = Object.freeze({ ...scope });
    const reject = (): never => {
      throw new Error('work_port_rejected');
    };
    const check = (context: HostCallContext) => {
      context.assertOpen();
      lease.assertOpen();
      lease.signal.throwIfAborted();
      if (
        this.closed ||
        context.scope.tenant !== bound.tenant ||
        context.scope.subject !== bound.subject ||
        context.scope.product !== bound.product
      )
        return reject();
    };
    const revalidate = witness.revalidate.bind(witness),
      acquire = witness.acquire.bind(witness);
    this.handlers = {
      'authority.revalidate': async (_raw, ctx) => {
        check(ctx);
        await revalidate(ctx.signal);
        check(ctx);
        return new Uint8Array();
      },
      'lease.acquire': async (_raw, ctx) => {
        check(ctx);
        const release = await acquire(ctx.signal);
        try {
          check(ctx);
          if (typeof release !== 'function' || this.releases.size >= 256) return reject();
          const id = newHostPortID();
          if (this.releases.has(id)) return reject();
          this.releases.set(id, release);
          return encodeHostJSON({ lease: id });
        } catch (error) {
          if (typeof release === 'function') await release();
          throw error;
        }
      },
      'lease.release': async (raw, ctx) => {
        check(ctx);
        const value = decodeHostObject(raw, reject),
          id = value.lease;
        if (typeof id !== 'string' || !/^[0-9a-f]{64}$/u.test(id)) return reject();
        const release = this.releases.get(id);
        if (release) {
          this.releases.delete(id);
          await release();
        }
        check(ctx);
        return new Uint8Array();
      },
    };
  }
  async close(): Promise<void> {
    this.closed = true;
    const releases = [...this.releases.values()];
    this.releases.clear();
    const results = await Promise.allSettled(
      releases.map(async (release) => {
        await release();
      }),
    );
    const errors = results.filter(
      (result): result is PromiseRejectedResult => result.status === 'rejected',
    );
    if (errors.length)
      throw new AggregateError(
        errors.map((result): unknown => result.reason),
        'work_port_cleanup_failed',
      );
  }
}
