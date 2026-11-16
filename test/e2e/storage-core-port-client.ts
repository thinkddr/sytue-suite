/* SPDX-License-Identifier: Apache-2.0 */
import type { StorageRepository } from '../../apps/frontend/packages/offline/src/matrix/storage-repository.js';
import { PqcStoragePort } from '../../apps/frontend/packages/offline/src/pqc/storage-port.js';
import type { PqcRuntime } from '../../apps/frontend/packages/offline/src/pqc/runtime.js';
import type { HostPortHandle } from '../../apps/frontend/packages/offline/src/pqc/host-port.js';
import { encodeStorageJSON } from '../../apps/frontend/packages/offline/src/matrix/storage.js';
import type { StorageFileRequest } from '../../apps/frontend/packages/offline/src/matrix/storage-file-codec.js';
import type { MatrixReference } from '../../apps/frontend/packages/offline/src/matrix/types.js';

export async function verifyCoreStoragePort(
  records: StorageRepository,
  runtime: PqcRuntime,
  source: MatrixReference,
  request: StorageFileRequest,
  originalRecord: { reference: MatrixReference; object: string; payload: string },
): Promise<{ reference: MatrixReference; request: StorageFileRequest }> {
  const scope = records.scope,
    life = new AbortController();
  let handle: HostPortHandle | undefined;
  const adapter = new PqcStoragePort(records, runtime, () => handle?.id ?? '');
  const assertOpen = (): void => {
    life.signal.throwIfAborted();
    records.assertOpen();
  };
  handle = runtime.registerHostPort({
    scope,
    lease: { signal: life.signal, assertOpen },
    run: (action, signal) => records.withJob(action, signal),
    handlers: adapter.handlers,
    cleanup: () => adapter.close(),
  });
  const encoder = new TextEncoder(),
    decoder = new TextDecoder();
  const call = async <T>(input: Record<string, unknown>): Promise<T> => {
    const raw = encoder.encode(JSON.stringify(input));
    try {
      const out = await runtime.invoke<Uint8Array>({
        operation: 'core.storage.call',
        port: handle!.id,
        scope,
        input: raw,
      });
      try {
        return JSON.parse(decoder.decode(out)) as T;
      } finally {
        out.fill(0);
      }
    } catch {
      throw new Error('shared-core-operation:' + String(input.action));
    } finally {
      raw.fill(0);
    }
  };
  try {
    const binding = await call<{ target: { room_id: string; device_id: string }; rooms: string[] }>(
      { action: 'prepare' },
    );
    if (!binding.rooms.includes(source.room_id)) throw new Error('core-foreign-binding');
    const read = await call<{ payload: string }>({
      action: 'record.read',
      reference: originalRecord.reference,
      object: originalRecord.object,
      role: 'content',
    });
    if (read.payload !== originalRecord.payload)
      throw new Error('core-changed-precise-native-record');
    const history = await call<{ Entries: Array<{ reference: MatrixReference }>; Next: string }>({
      action: 'history',
      room: source.room_id,
      cursor: '',
      limit: 100,
    });
    if (!Array.isArray(history.Entries) || !history.Entries.length)
      throw new Error('core-no-real-catalog-history');
    await call({
      action: 'file.verify',
      reference: source,
      object: request.object_id,
      revision: request.revision_id,
    });
    const destination = {
      ...request,
      object_id: 'go-port-copy',
      revision_id: 'go-port-copy-revision',
      metadata: encodeStorageJSON(
        encoder.encode('{"title":"exact copy through shared Go core","integer":9007199254740993}'),
      ),
    };
    const copied = await call<MatrixReference>({
      action: 'file.copy',
      reference: source,
      object: request.object_id,
      revision: request.revision_id,
      job: 'go-port-copy-job',
      target: binding.target,
      request: destination,
    });
    const copiedBytes = await records
      .files()
      .readRange(copied, destination.object_id, destination.revision_id, 0, request.size_bytes);
    try {
      if (copiedBytes.byteLength !== 73 || copiedBytes.some((byte) => byte !== 9))
        throw new Error('nested-go-io-changed-original');
    } finally {
      copiedBytes.fill(0);
    }
    const catalog = await call<{ reference: MatrixReference }>({
      action: 'object.put',
      target: binding.target,
      transaction: 'go-port-copy-catalog',
      object: destination.object_id,
      revision: 'go-port-catalog-revision',
      commit: {
        kind: 'org.sytue.object.v1',
        state: 'live',
        metadata: encodeStorageJSON(encoder.encode('{}')),
        content: { kind: 'file', revision_id: destination.revision_id, reference: copied },
      },
    });
    await call({ action: 'file.ack', job: 'go-port-copy-job', reference: catalog.reference });
    const saved = await records.writeContext().journal.read('j_go-port-copy-job');
    if (saved !== null) {
      saved.fill(0);
      throw new Error('go-ack-kept-file-retry-keys');
    }
    let denied = false;
    try {
      await runtime.invoke({
        operation: 'core.storage.call',
        port: handle.id,
        scope: { ...scope, subject: 'foreign' },
        input: encoder.encode('{"action":"bindings"}'),
      });
    } catch {
      denied = true;
    }
    if (!denied) throw new Error('go-storage-cross-scope-accepted');
    return { reference: copied, request: destination };
  } finally {
    handle.revoke();
    await handle.drain();
    life.abort();
  }
}
