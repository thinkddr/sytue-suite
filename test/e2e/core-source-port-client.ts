// SPDX-License-Identifier: Apache-2.0
import { PqcSourcePort } from '../../apps/frontend/packages/offline/src/pqc/source-port.js';
import { encodeHostJSON } from '../../apps/frontend/packages/offline/src/pqc/host-codec.js';
import type {
  HostScope,
  HostLease,
} from '../../apps/frontend/packages/offline/src/pqc/host-port.js';
import type { PqcRuntime } from '../../apps/frontend/packages/offline/src/pqc/runtime.js';
import type { StorageJobs } from '../../apps/frontend/packages/offline/src/matrix/storage-jobs.js';

export async function verifyCoreSourcePort(
  runtime: PqcRuntime,
  scope: HostScope,
  jobs: StorageJobs,
  lease: HostLease,
): Promise<void> {
  const original = new Uint8Array((2 << 20) + 17).fill(97);
  const sum = new Uint8Array(await crypto.subtle.digest('SHA-256', original));
  const sha256 = Array.from(sum, (byte) => byte.toString(16).padStart(2, '0')).join('');
  const epoch = '018f3340-7b12-7000-8000-000000000001';
  const row = { kind: 'mime_message', id: epoch, size: original.length, sha256 };
  original.fill(0);
  sum.fill(0);
  let reads = 0,
    portReads = 0;
  const source = new PqcSourcePort(scope, lease, {
    state: async () => {
      throw new Error('state not selected in source byte proof');
    },
    begin: async () => {
      throw new Error('begin not selected in source byte proof');
    },
    page: async () => {
      throw new Error('page not selected in source byte proof');
    },
    open: async (_epoch, _row, signal) => {
      reads++;
      return fetch('/source-original', { signal, redirect: 'error', credentials: 'omit' });
    },
  });
  const port = runtime.registerHostPort({
    scope,
    lease,
    run: (action, signal) => jobs.run(crypto.randomUUID(), action, signal),
    handlers: {
      ...source.handlers,
      'source.read': async (raw, context) => {
        portReads++;
        try {
          return await source.handlers['source.read']!(raw, context);
        } catch (error) {
          Object.assign(window, {
            coreSourceReadError: {
              message: error instanceof Error ? error.message : 'unknown',
              port_reads: portReads,
            },
          });
          throw error;
        }
      },
    },
    cleanup: () => source.close(),
  });
  const invoke = async (input: unknown, requested = scope) => {
    const bytes = encodeHostJSON(input);
    try {
      return await runtime.invoke<Uint8Array<ArrayBuffer>>({
        operation: 'core.source.call',
        port: port.id,
        scope: requested,
        input: bytes,
      });
    } finally {
      bytes.fill(0);
    }
  };
  try {
    const result = await invoke({ action: 'verify', epoch, row });
    const verified = JSON.parse(new TextDecoder().decode(result));
    result.fill(0);
    if (verified.sha256 !== sha256 || verified.size !== row.size || Number(reads) !== 1)
      throw new Error('source-Go-stream-exact');
    let rejected = false;
    try {
      await invoke({ action: 'verify', epoch, row: { ...row, sha256: '0'.repeat(64) } });
    } catch {
      rejected = true;
    }
    if (!rejected || Number(reads) !== 2) throw new Error('source-Go-stream-integrity-bypass');
    rejected = false;
    try {
      await invoke({ action: 'verify', epoch, row }, { ...scope, subject: 'another' });
    } catch {
      rejected = true;
    }
    if (!rejected || Number(reads) !== 2)
      throw new Error('source-Go-foreign-scope-reached-network');
    Object.assign(window, {
      coreSourceProof: { port_reads: portReads, network_reads: reads, size_bytes: row.size },
    });
  } finally {
    port.revoke();
    await port.drain();
  }
}
