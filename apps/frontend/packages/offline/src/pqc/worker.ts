/* SPDX-License-Identifier: Apache-2.0 */
import { ownedTransfers, eraseOwnedBytes } from './owned-bytes.js';
import {
  isClientCoreModule,
  clientCoreModuleBounds,
  type ClientCoreModule,
} from '@sytue/config/client-core';
import { hostMethodBounds, type HostCall, type HostMethod, type HostScope } from './host-port.js';

interface GoRuntime {
  env: Record<string, string>;
  importObject: WebAssembly.Imports;
  run: (instance: WebAssembly.Instance) => Promise<void>;
}
interface CoreResult {
  ok: boolean;
  value?: unknown;
  code?: string;
}
interface CryptoCore {
  version: number;
  module_id?: string;
  invoke: (request: Record<string, unknown>) => CoreResult | Promise<CoreResult>;
  cancel?: (id: number) => void;
  revokePort?: (id: string) => Promise<boolean>;
}
const host = globalThis as unknown as {
  Go: new () => GoRuntime;
  sytuePqc?: CryptoCore;
  sytueCoreHostCall?: (request: {
    port: string;
    scope: HostScope | null;
    method: HostMethod;
    payload: Uint8Array<ArrayBuffer>;
  }) => Promise<Uint8Array<ArrayBuffer>>;
  postMessage: (value: unknown, transfer?: Transferable[]) => void;
  onmessage: ((event: MessageEvent<unknown>) => void) | null;
};

let nextHost = 0;
const hostPending = new Map<
  number,
  {
    port: string;
    maximum: number;
    resolve: (value: Uint8Array<ArrayBuffer>) => void;
    reject: (error: Error) => void;
  }
>();
const retiredPorts = new Set<string>();
host.sytueCoreHostCall = (request) =>
  new Promise((resolve, reject) => {
    if (
      !request ||
      !/^[0-9a-f]{64}$/.test(request.port) ||
      retiredPorts.has(request.port) ||
      !Object.hasOwn(hostMethodBounds, request.method) ||
      !(request.payload instanceof Uint8Array) ||
      !(request.payload.buffer instanceof ArrayBuffer)
    ) {
      reject(new Error('host_port_rejected'));
      return;
    }
    const bounds = hostMethodBounds[request.method];
    if (
      request.payload.byteLength > bounds[0] ||
      nextHost >= Number.MAX_SAFE_INTEGER ||
      hostPending.size >= 128
    ) {
      request.payload.fill(0);
      reject(new Error('host_port_rejected'));
      return;
    }
    const id = ++nextHost;
    hostPending.set(id, { port: request.port, maximum: bounds[1], resolve, reject });
    try {
      const call: HostCall = { type: 'host.call', id, ...request };
      host.postMessage(call, [request.payload.buffer]);
    } catch {
      hostPending.delete(id);
      request.payload.fill(0);
      reject(new Error('host_port_rejected'));
    }
  });

let initialize: ((value: { url: URL; module: ClientCoreModule }) => void) | null = null;
const assets = new Promise<{ url: URL; module: ClientCoreModule }>((resolve) => {
  initialize = resolve;
});

const ready = (async (): Promise<CryptoCore> => {
  // The compiler emits both assets from the same pinned Go toolchain. The
  // browser loads a maintained runtime, not a second crypto implementation.
  const { url: assetBase, module } = await assets;
  const artifact = (name: string, digest?: string): URL => {
    const url = new URL(name, assetBase);
    if (module !== 'core') url.searchParams.set('module', module);
    if (digest) url.searchParams.set('sha256', digest);
    return url;
  };
  const manifestResponse = await fetch(artifact('manifest.json'), {
    credentials: 'omit',
    redirect: 'error',
    cache: 'no-cache',
  });
  if (!manifestResponse.ok) throw new Error('crypto_runtime_unavailable');
  const manifest = (await manifestResponse.json()) as {
    version?: unknown;
    module_id?: unknown;
    wasm_sha256?: unknown;
    runtime_sha256?: unknown;
  };
  if (
    manifest.version !== 1 ||
    (manifest.module_id !== module && !(module === 'core' && manifest.module_id === undefined)) ||
    typeof manifest.wasm_sha256 !== 'string' ||
    !/^[0-9a-f]{64}$/.test(manifest.wasm_sha256) ||
    typeof manifest.runtime_sha256 !== 'string' ||
    !/^[0-9a-f]{64}$/.test(manifest.runtime_sha256)
  )
    throw new Error('crypto_runtime_unavailable');
  const goRuntimeURL = artifact('wasm_exec.js', manifest.runtime_sha256).href;
  await import(/* @vite-ignore */ goRuntimeURL);
  const go = new host.Go();
  go.env = { ...go.env, GOMAXPROCS: '1' };
  const response = await fetch(artifact('core.wasm', manifest.wasm_sha256), {
    credentials: 'omit',
    redirect: 'error',
  });
  if (!response.ok) throw new Error('crypto_runtime_unavailable');
  const declared = Number(response.headers.get('content-length'));
  if (Number.isFinite(declared) && declared > clientCoreModuleBounds[module])
    throw new Error('crypto_runtime_unavailable');
  const binary = await response.arrayBuffer();
  if (binary.byteLength > clientCoreModuleBounds[module])
    throw new Error('crypto_runtime_unavailable');
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', binary));
  const actual = Array.from(digest, (byte) => byte.toString(16).padStart(2, '0')).join('');
  if (actual !== manifest.wasm_sha256) throw new Error('crypto_runtime_unavailable');
  const { instance } = await WebAssembly.instantiate(binary, go.importObject);
  void go.run(instance).catch(() => host.postMessage({ type: 'failed' }));
  await Promise.resolve();
  if (
    host.sytuePqc?.version !== 1 ||
    (host.sytuePqc.module_id !== module &&
      !(module === 'core' && host.sytuePqc.module_id === undefined))
  )
    throw new Error('crypto_runtime_unavailable');
  return host.sytuePqc;
})();
void ready
  .then((core) => host.postMessage({ type: 'ready', module_id: core.module_id ?? 'core' }))
  .catch(() => host.postMessage({ type: 'failed' }));

host.onmessage = (event): void => {
  const control = event.data as {
    type?: unknown;
    id?: unknown;
    port?: unknown;
    ok?: unknown;
    value?: unknown;
  } | null;
  if (control?.type === 'host.result') {
    if (typeof control.id !== 'number' || !Number.isSafeInteger(control.id)) return;
    const entry = hostPending.get(control.id);
    if (!entry || entry.port !== control.port) {
      if (control.value instanceof Uint8Array) control.value.fill(0);
      return;
    }
    hostPending.delete(control.id);
    if (
      control.ok !== true ||
      retiredPorts.has(entry.port) ||
      !(control.value instanceof Uint8Array) ||
      !(control.value.buffer instanceof ArrayBuffer) ||
      control.value.byteLength > entry.maximum
    ) {
      if (control.value instanceof Uint8Array) control.value.fill(0);
      entry.reject(new Error('host_port_rejected'));
    } else {
      entry.resolve(control.value.slice());
      control.value.fill(0);
    }
    return;
  }
  if (control?.type === 'host.revoke' && typeof control.port === 'string') {
    if (!/^[0-9a-f]{64}$/.test(control.port)) return;
    const port = control.port;
    retiredPorts.add(port);
    for (const [id, entry] of hostPending) {
      if (entry.port !== control.port) continue;
      hostPending.delete(id);
      entry.reject(new Error('host_port_rejected'));
    }
    void ready
      .then(async (core) => (core.revokePort ? await core.revokePort(port) : false))
      .then(
        (ok) => host.postMessage({ type: 'host.revoked', port, ok }),
        () => host.postMessage({ type: 'host.revoked', port, ok: false }),
      );
    return;
  }
  if (
    control?.type === 'cancel' &&
    typeof control.id === 'number' &&
    Number.isSafeInteger(control.id) &&
    control.id > 0
  ) {
    host.sytuePqc?.cancel?.(control.id);
    return;
  }
  const bootstrap = event.data as {
    type?: unknown;
    assetBaseURL?: unknown;
    module?: unknown;
  } | null;
  if (bootstrap?.type === 'initialize') {
    try {
      if (
        !initialize ||
        typeof bootstrap.assetBaseURL !== 'string' ||
        !isClientCoreModule(bootstrap.module)
      )
        throw new Error();
      const url = new URL(bootstrap.assetBaseURL);
      if (
        url.origin !== globalThis.location.origin ||
        !url.pathname.endsWith('/api/client-core/') ||
        url.search ||
        url.hash
      )
        throw new Error();
      initialize({ url, module: bootstrap.module });
      initialize = null;
    } catch {
      host.postMessage({ type: 'failed' });
    }
    return;
  }
  const request = event.data as { id?: unknown; request?: unknown } | null;
  if (
    !request ||
    typeof request.id !== 'number' ||
    !Number.isSafeInteger(request.id) ||
    !request.request ||
    typeof request.request !== 'object'
  )
    return;
  const id = request.id;
  void ready
    .then(async (core) => {
      const result = await core.invoke({
        ...(request.request as Record<string, unknown>),
        _request_id: id,
      });
      const response = { type: 'result', id, ...result };
      host.postMessage(response, ownedTransfers(result.value));
    })
    .catch(() => host.postMessage({ type: 'result', id, ok: false, code: 'crypto_rejected' }))
    .finally(() => eraseOwnedBytes(request.request));
};
