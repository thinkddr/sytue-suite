/* SPDX-License-Identifier: Apache-2.0 */
import { eraseOwnedBytes } from './owned-bytes.js';
import { withBase } from '@sytue/config/base-path';
import { isClientCoreModule, type ClientCoreModule } from '@sytue/config/client-core';
export type { ClientCoreModule } from '@sytue/config/client-core';
import {
  PqcHostPorts,
  checkedHostBinding,
  type HostBinding,
  type HostCall,
  type HostPortHandle,
  type HostPortOptions,
  type HostDelegationResolver,
} from './host-port.js';
export interface PqcEnvelope {
  v: 1;
  profile: string;
  key_id: string;
  ciphertext: Uint8Array;
}
export interface PqcIdentityMaterial {
  profile: string;
  recipient: string;
  key_id: string;
  identity: Uint8Array;
}
export interface PqcLease {
  readonly binding?: Readonly<HostBinding>;
  signal: AbortSignal;
  assertOpen: () => void;
}
interface Pending {
  port?: string;
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
  cleanup: () => void;
}

/** One worker per authorized account/product lease. It contains no network
 * credentials or persistence, and all algorithms execute in the shared Go core. */
export class PqcRuntime {
  private worker: Worker | null = null;
  private readiness: Promise<void> | null = null;
  private next = 0;
  private readonly pending = new Map<number, Pending>();
  private readonly binding: Readonly<HostBinding> | undefined;
  private readonly ports = new PqcHostPorts(
    () => this.assertOpen(),
    (port) => this.retirePort(port),
    () => this.binding,
    (capability) => {
      if (!this.delegationResolver) throw new Error('host_port_rejected');
      return this.delegationResolver(capability);
    },
  );
  private readonly retirements = new Map<
    string,
    { worker: Worker; resolve: () => void; reject: (error: Error) => void }
  >();
  private closed = false;
  private readyReject: ((error: Error) => void) | null = null;
  private readyTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly logout = (): void => {
    this.close();
  };
  constructor(
    private readonly lease: PqcLease,
    private readonly module: ClientCoreModule = 'core',
    private readonly delegationResolver?: HostDelegationResolver,
  ) {
    if (!isClientCoreModule(module)) throw new Error('crypto_runtime_unavailable');
    this.binding = lease.binding ? checkedHostBinding(lease.binding) : undefined;
    lease.signal.addEventListener('abort', this.logout, { once: true });
    if (lease.signal.aborted) this.close();
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.lease.signal.removeEventListener('abort', this.logout);
    this.reset(new DOMException('Aborted', 'AbortError'));
  }

  registerHostPort(options: HostPortOptions): HostPortHandle {
    return this.ports.register(options);
  }
  /** Only an opaque token minted by the runtime's SuiteMatrix owner can select
   * a different verified SDK binding. JSON objects and foreign tokens reject. */
  registerDelegatedHostPort(capability: object): HostPortHandle {
    return this.ports.registerDelegated(capability);
  }

  /** Owners await this boundary before erasing SDK keys or the encrypted cache. */
  async closeAndDrain(): Promise<void> {
    this.close();
    await this.ports.drainAll();
  }

  private retirePort(port: string): Promise<void> {
    const worker = this.worker;
    for (const [id, entry] of this.pending) {
      if (entry.port !== port) continue;
      this.pending.delete(id);
      entry.cleanup();
      entry.reject(new DOMException('Aborted', 'AbortError'));
      try {
        worker?.postMessage({ type: 'cancel', id });
      } catch {
        /* Retired. */
      }
    }
    if (!worker) return Promise.resolve();
    return new Promise<void>((resolve, reject) => {
      this.retirements.set(port, { worker, resolve, reject });
      try {
        worker.postMessage({ type: 'host.revoke', port });
      } catch {
        this.reset(new Error('crypto_runtime_unavailable'));
      }
    });
  }
  private assertOpen(): void {
    if (this.closed || this.lease.signal.aborted) throw new DOMException('Aborted', 'AbortError');
    this.lease.assertOpen();
  }

  private reset(error: Error): void {
    this.ports.revokeAll();
    const worker = this.worker;
    this.worker = null;
    this.readiness = null;
    worker?.terminate();
    // Termination destroys all Go memory. Actual host IO is still joined by
    // PqcHostPorts before cleanup; no worker ACK can arrive after termination.
    for (const [port, entry] of this.retirements) {
      if (entry.worker === worker) {
        this.retirements.delete(port);
        entry.resolve();
      }
    }
    const rejectReady = this.readyReject;
    this.readyReject = null;
    if (this.readyTimer) clearTimeout(this.readyTimer);
    this.readyTimer = null;
    rejectReady?.(error);
    for (const entry of this.pending.values()) {
      entry.cleanup();
      entry.reject(error);
    }
    this.pending.clear();
  }

  private ensureWorker(): Promise<void> {
    this.assertOpen();
    if (this.readiness) return this.readiness;
    const worker = new Worker(new URL('./worker.ts', import.meta.url), {
      type: 'module',
      name: this.module === 'core' ? 'sytue-crypto' : 'sytue-' + this.module,
    });
    this.worker = worker;
    this.readiness = new Promise<void>((resolve, reject) => {
      this.readyReject = reject;
      const failed = (): void => {
        reject(new Error('crypto_runtime_unavailable'));
        if (this.worker === worker) this.reset(new Error('crypto_runtime_unavailable'));
      };
      this.readyTimer = setTimeout(failed, 15000);
      worker.onerror = failed;
      worker.onmessageerror = failed;
      worker.onmessage = (
        event: MessageEvent<{
          type?: string;
          id?: number;
          ok?: boolean;
          value?: unknown;
          module_id?: unknown;
        }>,
      ): void => {
        if (this.worker !== worker) return;
        const message = event.data;
        if (message.type === 'host.revoked') {
          const port = (message as { port?: unknown }).port;
          if (typeof port !== 'string') return;
          const entry = this.retirements.get(port);
          if (!entry || entry.worker !== worker) return;
          this.retirements.delete(port);
          if (message.ok === true) entry.resolve();
          else {
            entry.reject(new Error('crypto_runtime_unavailable'));
            this.reset(new Error('crypto_runtime_unavailable'));
          }
          return;
        }
        if (message.type === 'host.call') {
          const call = message as unknown as HostCall;
          const currentWorker = (): void => {
            if (this.worker !== worker) throw new DOMException('Aborted', 'AbortError');
            this.assertOpen();
          };
          void this.ports
            .dispatch(call, currentWorker)
            .then(
              (value) => {
                try {
                  currentWorker();
                  worker.postMessage(
                    { type: 'host.result', id: call.id, port: call.port, ok: true, value },
                    [value.buffer],
                  );
                } catch {
                  value.fill(0);
                }
              },
              () => {
                if (this.worker !== worker) return;
                try {
                  worker.postMessage({
                    type: 'host.result',
                    id: call.id,
                    port: call.port,
                    ok: false,
                  });
                } catch {
                  /* Retired. */
                }
              },
            )
            .finally(() => {
              if (call.payload instanceof Uint8Array) call.payload.fill(0);
            });
          return;
        }
        if (message.type === 'failed') {
          failed();
          return;
        }
        if (message.type === 'ready') {
          if (message.module_id !== this.module) {
            failed();
            return;
          }
          this.readyReject = null;
          if (this.readyTimer) clearTimeout(this.readyTimer);
          this.readyTimer = null;
          resolve();
          return;
        }
        if (message.type !== 'result' || !Number.isSafeInteger(message.id)) return;
        const entry = this.pending.get(message.id!);
        if (!entry) {
          eraseOwnedBytes(message.value);
          return;
        }
        this.pending.delete(message.id!);
        entry.cleanup();
        try {
          this.assertOpen();
          if (entry.port) this.ports.assertLive(entry.port);
          if (message.ok !== true) throw new Error('crypto_rejected');
          entry.resolve(message.value);
        } catch (error) {
          eraseOwnedBytes(message.value);
          entry.reject(error instanceof Error ? error : new Error('crypto_rejected'));
        }
      };
      const base = (import.meta as { env?: { BASE_URL?: string } }).env?.BASE_URL ?? '/';
      const assetBaseURL = new URL(withBase(base, '/api/client-core/'), globalThis.location.origin);
      worker.postMessage({
        type: 'initialize',
        assetBaseURL: assetBaseURL.href,
        module: this.module,
      });
    });
    return this.readiness;
  }

  /** Requests carry byte arrays, preserving precise JSON and keeping private
   * material out of URLs, logs, strings and browser storage. Cancellation retires
   * this worker's memory; a later request starts a fresh one using sealed keys. */
  async invoke<T>(request: Record<string, unknown>, signal?: AbortSignal): Promise<T> {
    this.assertOpen();
    const port = typeof request.port === 'string' ? request.port : undefined;
    if (port) this.ports.assertLive(port);
    signal?.throwIfAborted();
    const abortStarting = (): void => this.reset(new DOMException('Aborted', 'AbortError'));
    signal?.addEventListener('abort', abortStarting, { once: true });
    try {
      await this.ensureWorker();
    } finally {
      signal?.removeEventListener('abort', abortStarting);
    }
    this.assertOpen();
    signal?.throwIfAborted();
    if (port) this.ports.assertLive(port);
    const worker = this.worker;
    if (!worker) throw new Error('crypto_runtime_unavailable');
    const id = ++this.next;
    return new Promise<T>((resolve, reject) => {
      const abort = (): void => {
        if (this.worker === worker) this.reset(new DOMException('Aborted', 'AbortError'));
      };
      const cleanup = (): void => signal?.removeEventListener('abort', abort);
      this.pending.set(id, {
        ...(port ? { port } : {}),
        resolve: (value) => resolve(value as T),
        reject,
        cleanup,
      });
      signal?.addEventListener('abort', abort, { once: true });
      if (signal?.aborted) {
        abort();
        return;
      }
      try {
        worker.postMessage({ id, request });
      } catch {
        this.reset(new Error('crypto_runtime_unavailable'));
      }
    });
  }
}
