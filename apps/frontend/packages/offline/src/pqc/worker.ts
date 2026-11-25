/* SPDX-License-Identifier: Apache-2.0 */
import { ownedTransfers, eraseOwnedBytes } from './owned-bytes.js';
import {
  isClientCoreModule,
  clientCoreModuleBounds,
  vaultCoreModule,
  type ClientCoreModule,
} from '@sytue/config/client-core';
import { hostMethodBounds, type HostCall, type HostMethod, type HostScope } from './host-port.js';
import { isOwnerWorkerMessage } from './worker-origin.js';

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

interface Digests {
  wasm: string;
  runtime: string;
}
const hex64 = /^[0-9a-f]{64}$/;

let initialize:
  ((value: { url: URL; module: ClientCoreModule; digests: Digests | null }) => void) | null = null;
const assets = new Promise<{ url: URL; module: ClientCoreModule; digests: Digests | null }>(
  (resolve) => {
    initialize = resolve;
  },
);

/** Que el documento sepa que la descarga avanza: su plazo es de silencio, no total. */
let lastProgress = 0;
function progress(): void {
  const now = Date.now();
  if (now - lastProgress < 1000) return;
  lastProgress = now;
  host.postMessage({ type: 'progress' });
}

/**
 * Baja el wasm, con su tope de tamaño, y comprueba su sha256 antes de devolver un
 * solo byte: lo que se instancia es exactamente lo que el compilador publicó.
 * `same-origin` y no `omit`: es como pide la página su `<link rel=preload
 * crossorigin>`, y con otro modo de credenciales la caché no los junta (la ruta del
 * BFF no lee la sesión: `clientCoreProxy`).
 */
async function verifiedWasm(url: URL, digest: string, maximum: number): Promise<ArrayBuffer> {
  const response = await fetch(url, { credentials: 'same-origin', redirect: 'error' });
  if (!response.ok || !response.body) throw new Error('crypto_runtime_unavailable');
  const declared = Number(response.headers.get('content-length'));
  if (Number.isFinite(declared) && declared > maximum)
    throw new Error('crypto_runtime_unavailable');
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > maximum) {
      await reader.cancel().catch(() => {});
      throw new Error('crypto_runtime_unavailable');
    }
    chunks.push(value);
    progress();
  }
  const binary = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    binary.set(chunk, offset);
    offset += chunk.byteLength;
  }
  const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', binary));
  const actual = Array.from(hash, (byte) => byte.toString(16).padStart(2, '0')).join('');
  if (actual !== digest) throw new Error('crypto_runtime_unavailable');
  return binary.buffer;
}

const ready = (async (): Promise<CryptoCore> => {
  // The compiler emits both assets from the same pinned Go toolchain. The
  // browser loads a maintained runtime, not a second crypto implementation.
  const { url: assetBase, module: requested, digests } = await assets;
  let module: ClientCoreModule = requested;
  const artifact = (name: string, digest?: string): URL => {
    const url = new URL(name, assetBase);
    if (module !== 'core') url.searchParams.set('module', module);
    if (digest) url.searchParams.set('sha256', digest);
    return url;
  };
  const fromManifest = async (): Promise<Digests> => {
    const fetchManifest = (): Promise<Response> =>
      fetch(artifact('manifest.json'), {
        credentials: 'omit',
        redirect: 'error',
        cache: 'no-cache',
      });
    let manifestResponse = await fetchManifest();
    // Un Rooms anterior al núcleo de la bóveda contesta 400 a `vault`. El núcleo
    // completo lleva las mismas operaciones de la bóveda (es su superconjunto, con la
    // misma huella comprobada), así que se carga ese en vez de dejar la app sin bóveda.
    if (manifestResponse.status === 400 && module === vaultCoreModule) {
      module = 'core';
      manifestResponse = await fetchManifest();
    }
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
      !hex64.test(manifest.wasm_sha256) ||
      typeof manifest.runtime_sha256 !== 'string' ||
      !hex64.test(manifest.runtime_sha256)
    )
      throw new Error('crypto_runtime_unavailable');
    return { wasm: manifest.wasm_sha256, runtime: manifest.runtime_sha256 };
  };
  // Las dos descargas A LA VEZ (antes: manifiesto, luego `wasm_exec.js`, luego el
  // wasm, tres viajes en serie). El servidor solo sirve un `sha256` que sea el suyo.
  const load = async (pinned: Digests): Promise<ArrayBuffer> => {
    const runtime = import(/* @vite-ignore */ artifact('wasm_exec.js', pinned.runtime).href);
    const binary = verifiedWasm(
      artifact('core.wasm', pinned.wasm),
      pinned.wasm,
      clientCoreModuleBounds[module],
    );
    // Que el rechazo del que no se espere no quede sin atender.
    runtime.catch(() => {});
    binary.catch(() => {});
    await runtime;
    return binary;
  };
  let binary: ArrayBuffer;
  if (digests) {
    try {
      binary = await load(digests);
    } catch {
      // Los resúmenes de la página ya no son los del servidor (se desplegó otro
      // núcleo desde que se pintó): se le preguntan, una vez.
      const fresh = await fromManifest();
      if (fresh.wasm === digests.wasm && fresh.runtime === digests.runtime)
        throw new Error('crypto_runtime_unavailable');
      binary = await load(fresh);
    }
  } else {
    binary = await load(await fromManifest());
  }
  const go = new host.Go();
  go.env = { ...go.env, GOMAXPROCS: '1' };
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

// Worker dedicado: solo el documento que lo creó puede enviarle mensajes. Se
// comprueba igualmente el origen y, después, la forma de cada mensaje por tipo.
host.onmessage = (event): void => {
  if (!isOwnerWorkerMessage(event, globalThis.location.origin)) return;
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
    digests?: unknown;
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
      const pinned = bootstrap.digests as {
        wasm_sha256?: unknown;
        runtime_sha256?: unknown;
      } | null;
      const digests =
        bootstrap.module === 'core' &&
        pinned &&
        typeof pinned === 'object' &&
        typeof pinned.wasm_sha256 === 'string' &&
        hex64.test(pinned.wasm_sha256) &&
        typeof pinned.runtime_sha256 === 'string' &&
        hex64.test(pinned.runtime_sha256)
          ? { wasm: pinned.wasm_sha256, runtime: pinned.runtime_sha256 }
          : null;
      initialize({ url, module: bootstrap.module, digests });
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
