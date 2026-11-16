/* SPDX-License-Identifier: Apache-2.0 */
export interface HostScope {
  readonly tenant: string;
  readonly subject: string;
  readonly product: string;
}
export interface HostBinding {
  readonly scope: Readonly<HostScope>;
  readonly matrixUserID: string;
  readonly deviceID: string;
}
export interface HostLease {
  readonly signal: AbortSignal;
  assertOpen: () => void;
}
/** Mechanical boundary schema, checked against webport.Contract in the real Wasm proof. */
export const hostMethodBounds = Object.freeze({
  current: [0, 64 << 10],
  'identity.root': [0, 4096],
  'lease.acquire': [4096, 4096],
  'lease.release': [4096, 4096],
  'authority.revalidate': [4096, 4096],
  'journal.read': [8192, (8 << 20) + 1],
  'journal.write': [(8 << 20) + 8192, 4096],
  'journal.delete': [8192, 4096],
  'journal.keys': [8192, 1 << 20],
  'journal.lock': [8192, 4096],
  'journal.unlock': [8192, 4096],
  'storage.prepare': [8192, 64 << 10],
  'storage.bindings': [0, 64 << 10],
  'storage.frozen': [0, 256 << 10],
  'storage.recoverable': [8192, 4096],
  'storage.target': [8192, 4096],
  'storage.record.read': [64 << 10, 1 << 20],
  'storage.record.put': [1 << 20, 64 << 10],
  'storage.object.read': [64 << 10, 1 << 20],
  'storage.object.put': [1 << 20, 1 << 20],
  'storage.history': [64 << 10, 8 << 20],
  'storage.file.info': [64 << 10, 64 << 10],
  'storage.file.commitment': [64 << 10, 64 << 10],
  'storage.file.download': [64 << 10, 64 << 10],
  'storage.file.read': [64 << 10, 256 << 10],
  'storage.file.close': [8192, 4096],
  'storage.file.upload': [64 << 10, 64 << 10],
  'storage.file.ack': [64 << 10, 4096],
  'storage.file.copy': [64 << 10, 64 << 10],
  'ingress.secret.read': [8192, 2049],
  'ingress.secret.write': [8192, 4096],
  'ingress.action.sign': [16 << 10, 16 << 10],
  'service.request': [320 << 10, 64 << 10],
  'service.read': [8192, 256 << 10],
  'service.close': [8192, 4096],
  'source.capabilities': [8192, 64 << 10],
  'source.begin': [8192, 64 << 10],
  'source.page': [64 << 10, 1 << 20],
  'source.open': [64 << 10, 64 << 10],
  'source.read': [64 << 10, 256 << 10],
  'source.close': [8192, 4096],
  'cutover.state': [8192, 64 << 10],
  'cutover.begin': [8192, 64 << 10],
  'cutover.complete': [64 << 10, 64 << 10],
  'submission.register': [64 << 10, 64 << 10],
  'submission.control': [8192, 64 << 10],
  'submission.receipt': [8192, 64 << 10],
  'submission.send': [64 << 10, 64 << 10],
} as const);
export type HostMethod = keyof typeof hostMethodBounds;
export interface HostCallContext {
  readonly scope: Readonly<HostScope>;
  readonly signal: AbortSignal;
  assertOpen: () => void;
}
export type HostHandler = (
  payload: Uint8Array<ArrayBuffer>,
  context: HostCallContext,
) => Promise<Uint8Array<ArrayBuffer>>;
export interface HostPortOptions {
  readonly scope: HostScope;
  readonly lease: HostLease;
  /** Must retain the maintained SDK owner job until the actual IO settles. */
  readonly run: <T>(action: (signal: AbortSignal) => Promise<T>, signal: AbortSignal) => Promise<T>;
  readonly handlers: Partial<Record<Exclude<HostMethod, 'current'>, HostHandler>>;
  /** Release retained locks/streams only after active IO has joined. */
  readonly cleanup: () => Promise<void>;
}
/** Opaque grants are resolved only by the owning SuiteMatrix closure. This
 * describes the resolver result, never a consumer-controlled port option. */
export interface ResolvedHostDelegation {
  readonly binding: Readonly<HostBinding>;
  readonly options: HostPortOptions;
}
export type HostDelegationResolver = (capability: object) => ResolvedHostDelegation;
export interface HostPortHandle {
  readonly id: string;
  readonly scope: Readonly<HostScope>;
  revoke: () => void;
  drain: () => Promise<void>;
}
export interface HostCall {
  type: 'host.call';
  id: number;
  port: string;
  scope: HostScope | null;
  method: HostMethod;
  payload: Uint8Array<ArrayBuffer>;
}
function rejected(): Error {
  return new Error('host_port_rejected');
}
function sameScope(left: HostScope | null, right: HostScope): boolean {
  return (
    !!left &&
    left.tenant === right.tenant &&
    left.subject === right.subject &&
    left.product === right.product
  );
}
function checkedScope(scope: HostScope): Readonly<HostScope> {
  const encoder = new TextEncoder();
  if (
    !scope ||
    ['tenant', 'subject', 'product'].some((key) => {
      const value = scope[key as keyof HostScope];
      return (
        typeof value !== 'string' ||
        !value ||
        encoder.encode(value).length > 16384 ||
        hasLineBreakOrNul(value)
      );
    })
  )
    throw rejected();
  return Object.freeze({
    tenant: scope.tenant,
    subject: scope.subject,
    product: scope.product,
  });
}
interface Entry {
  readonly binding: Readonly<HostBinding>;
  readonly scope: Readonly<HostScope>;
  readonly options: HostPortOptions;
  readonly handlers: HostPortOptions['handlers'];
  readonly controller: AbortController;
  readonly active: Set<Promise<void>>;
  readonly seen: Set<number>;
  revoked: boolean;
  drain: Promise<void> | null;
  detach: () => void;
}
export class PqcHostPorts {
  private readonly entries = new Map<string, Entry>();
  constructor(
    private readonly assertRuntime: () => void,
    private readonly retired: (id: string) => void | Promise<void>,
    private readonly binding: () => Readonly<HostBinding> | undefined,
    private readonly delegation?: HostDelegationResolver,
  ) {}
  register(options: HostPortOptions): HostPortHandle {
    return this.registerResolved(options, this.binding());
  }
  registerDelegated(capability: object): HostPortHandle {
    this.assertRuntime();
    if (!this.delegation || capability === null || typeof capability !== 'object') throw rejected();
    const resolved = this.delegation(capability),
      owner = this.binding();
    const binding = checkedHostBinding(resolved.binding);
    if (!owner || !sameScope(binding.scope, owner.scope)) throw rejected();
    return this.registerResolved(resolved.options, binding);
  }
  private registerResolved(
    options: HostPortOptions,
    resolvedBinding: Readonly<HostBinding> | undefined,
  ): HostPortHandle {
    this.assertRuntime();
    options.lease.assertOpen();
    if (
      options.lease.signal.aborted ||
      this.entries.size >= 32 ||
      typeof options.run !== 'function' ||
      typeof options.cleanup !== 'function'
    )
      throw rejected();
    const currentBinding = resolvedBinding;
    if (!currentBinding || !sameScope(options.scope, currentBinding.scope)) throw rejected();
    const binding = checkedHostBinding(currentBinding);
    const scope = checkedScope(binding.scope);
    options = Object.freeze({
      scope,
      lease: Object.freeze({
        signal: options.lease.signal,
        assertOpen: options.lease.assertOpen.bind(options.lease),
      }),
      run: options.run,
      handlers: options.handlers,
      cleanup: options.cleanup,
    });
    const handlers = Object.freeze({ ...options.handlers });
    for (const [method, handler] of Object.entries(handlers)) {
      if (
        method === 'current' ||
        !Object.hasOwn(hostMethodBounds, method) ||
        typeof handler !== 'function'
      )
        throw rejected();
    }
    const id = newHostPortID();
    if (this.entries.has(id)) throw rejected();
    const controller = new AbortController();
    const abort = (): void => this.revoke(id);
    const entry: Entry = {
      binding,
      scope,
      options,
      handlers,
      controller,
      active: new Set(),
      seen: new Set(),
      revoked: false,
      drain: null,
      detach: () => options.lease.signal.removeEventListener('abort', abort),
    };
    this.entries.set(id, entry);
    options.lease.signal.addEventListener('abort', abort, { once: true });
    if (options.lease.signal.aborted) this.revoke(id);
    return Object.freeze({
      id,
      scope,
      revoke: () => this.revoke(id),
      drain: () => this.drain(id),
    });
  }
  private check(entry: Entry): void {
    this.assertRuntime();
    if (entry.revoked || entry.controller.signal.aborted || entry.options.lease.signal.aborted)
      throw rejected();
    entry.options.lease.assertOpen();
  }
  async dispatch(input: HostCall, currentWorker: () => void): Promise<Uint8Array<ArrayBuffer>> {
    currentWorker();
    const method = input.method;
    const entry = this.entries.get(input.port);
    const bounds = Object.hasOwn(hostMethodBounds, method) ? hostMethodBounds[method] : null;
    if (
      !entry ||
      !bounds ||
      !Number.isSafeInteger(input.id) ||
      input.id < 1 ||
      !(input.payload instanceof Uint8Array) ||
      !(input.payload.buffer instanceof ArrayBuffer) ||
      input.payload.byteLength > bounds[0] ||
      entry.seen.has(input.id) ||
      entry.active.size >= 128 ||
      entry.seen.size >= 1_000_000 ||
      (method !== 'current' && !sameScope(input.scope, entry.scope))
    )
      throw rejected();
    this.check(entry);
    entry.seen.add(input.id);
    const task = (async (): Promise<Uint8Array<ArrayBuffer>> => {
      let output: Uint8Array<ArrayBuffer> | undefined;
      try {
        if (method === 'current') {
          // Routing metadata only. Sensitive operations use their scoped run guard.
          if (input.scope !== null && !sameScope(input.scope, entry.scope)) throw rejected();
          output = new TextEncoder().encode(
            JSON.stringify({
              v: 1,
              scope: entry.scope,
              binding: {
                matrix_user_id: entry.binding.matrixUserID,
                device_id: entry.binding.deviceID,
              },
            }),
          );
        } else
          output = await entry.options.run(async (signal) => {
            const assertOpen = (): void => {
              currentWorker();
              this.check(entry);
              signal.throwIfAborted();
            };
            assertOpen();
            const handler = entry.handlers[method];
            if (!handler) throw rejected();
            const value = await handler(input.payload, {
              scope: entry.scope,
              signal,
              assertOpen,
            });
            try {
              assertOpen();
              return value;
            } catch (error) {
              if (value instanceof Uint8Array) value.fill(0);
              throw error;
            }
          }, entry.controller.signal);
        currentWorker();
        this.check(entry);
        if (
          !(output instanceof Uint8Array) ||
          !(output.buffer instanceof ArrayBuffer) ||
          output.byteLength > bounds[1]
        )
          throw rejected();
        return output.slice();
      } finally {
        input.payload.fill(0);
        output?.fill(0);
      }
    })();
    const joined = task.then(
      () => {},
      () => {},
    );
    entry.active.add(joined);
    void joined.then(() => entry.active.delete(joined));
    return task;
  }
  assertLive(id: string): void {
    const entry = this.entries.get(id);
    if (!entry) throw rejected();
    this.check(entry);
  }
  revoke(id: string): void {
    const entry = this.entries.get(id);
    if (!entry || entry.revoked) return;
    let finish!: () => void;
    let fail!: (error: unknown) => void;
    entry.drain = new Promise<void>((resolve, reject) => {
      finish = resolve;
      fail = reject;
    });
    void entry.drain.catch(() => {});
    entry.revoked = true;
    entry.detach();
    entry.controller.abort();
    let retirement: Promise<void>;
    try {
      retirement = Promise.resolve(this.retired(id));
    } catch (error) {
      retirement = Promise.reject(error instanceof Error ? error : rejected());
    }
    void retirement.catch(() => {});
    void (async () => {
      while (entry.active.size > 0) await Promise.all(entry.active);
      let failed = false;
      try {
        await retirement;
      } catch {
        failed = true;
      }
      await entry.options.cleanup();
      this.entries.delete(id);
      if (failed) throw rejected();
    })().then(finish, fail);
  }
  async drain(id: string): Promise<void> {
    const entry = this.entries.get(id);
    if (!entry) return;
    if (!entry.revoked) throw rejected();
    await entry.drain;
  }
  revokeAll(): void {
    for (const id of this.entries.keys()) this.revoke(id);
  }
  async drainAll(): Promise<void> {
    this.revokeAll();
    const joined = await Promise.allSettled([...this.entries.keys()].map((id) => this.drain(id)));
    if (joined.some((result) => result.status === 'rejected')) throw rejected();
  }
}

// NUL, CR and LF would let a scope or binding field forge or split a framed
// value; checked without a regex literal so the intent is explicit.
function hasLineBreakOrNul(value: string): boolean {
  return value.includes('\u0000') || value.includes('\r') || value.includes('\n');
}

export function checkedHostBinding(binding: HostBinding): Readonly<HostBinding> {
  const scope = checkedScope(binding.scope);
  if (
    typeof binding.matrixUserID !== 'string' ||
    !binding.matrixUserID.startsWith('@') ||
    !binding.matrixUserID.includes(':') ||
    binding.matrixUserID.length > 2048 ||
    hasLineBreakOrNul(binding.matrixUserID) ||
    typeof binding.deviceID !== 'string' ||
    !binding.deviceID ||
    binding.deviceID.length > 255 ||
    hasLineBreakOrNul(binding.deviceID)
  )
    throw rejected();
  return Object.freeze({
    scope,
    matrixUserID: binding.matrixUserID,
    deviceID: binding.deviceID,
  });
}

/** Opaque scoped capabilities use the same unpredictable host-owned ID. */
export function newHostPortID(): string {
  const nonce = crypto.getRandomValues(new Uint8Array(32));
  const id = Array.from(nonce, (value) => value.toString(16).padStart(2, '0')).join('');
  nonce.fill(0);
  return id;
}
