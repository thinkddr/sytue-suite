/* SPDX-License-Identifier: Apache-2.0 */
import type { DriveClient } from '@sytue/api-clients';
import { MatrixBrowserAuth } from '@sytue/offline/matrix/auth';
import {
  MatrixSecurityError,
  PrivateDriveRepository,
  type SuiteMatrix,
  type StorageRepository,
} from '@sytue/offline/matrix';
import type { MatrixGateHost } from '@sytue/shell/matrix';

/** One private presenter per verified device; no client is ever rebound to another owner. */
export interface PrivateDriveSession {
  readonly client: SuiteMatrix;
  readonly storage: StorageRepository;
  readonly repository: PrivateDriveRepository;
  readonly signal: AbortSignal;
  readonly events: EventTarget;
  /** A retained upload needs attention. Provider messages and names stay out of notices. */
  uploadFailed: boolean;
  resume: () => Promise<void>;
}
interface Entry {
  value: PrivateDriveSession;
  ready: boolean;
  started: boolean;
}
export function createPrivateDriveGate(options: {
  actor: string;
  tenant?: string | undefined;
  /** Lazy: SSR never touches window, fetch or the SDK store. */
  source: () => DriveClient;
}) {
  const owner = Object.freeze({ actor: options.actor, tenant: options.tenant });
  const newSource = options.source;
  const auth = new MatrixBrowserAuth({ storageProduct: 'drive', deviceName: 'Sytue Drive · Web' });
  const entries = new Map<SuiteMatrix, Entry>();
  const current = (client: SuiteMatrix): PrivateDriveSession => {
    const entry = entries.get(client);
    if (!entry?.ready || client.isClosed || entry.value.signal.aborted)
      throw new MatrixSecurityError('device');
    return entry.value;
  };
  const host: MatrixGateHost = {
    bootstrap: auth.refreshBootstrap,
    begin: (bootstrap) => auth.begin(bootstrap),
    session: auth,
    prepare: async (client, signal) => {
      const bootstrap = client.bootstrap;
      if (
        bootstrap.actor !== owner.actor ||
        (owner.tenant !== undefined && bootstrap.tenant !== owner.tenant) ||
        bootstrap.storage_product !== 'drive'
      )
        throw new MatrixSecurityError('identity');
      signal.throwIfAborted();
      const source = newSource();
      const scope = Object.freeze({
        tenant: bootstrap.tenant,
        subject: owner.actor,
        product: 'drive' as const,
      });
      const storage = client.storage({
        read: (id, signal) => source.readCipherBlob(scope, id, signal),
        write: (id, bytes, signal) => source.putCipherBlob(scope, id, bytes, signal),
      });
      if (
        storage.scope.subject !== owner.actor ||
        storage.scope.tenant !== bootstrap.tenant ||
        storage.scope.product !== 'drive'
      )
        throw new MatrixSecurityError('identity');
      const repository = new PrivateDriveRepository(storage);
      const value: PrivateDriveSession = {
        client,
        storage,
        repository,
        signal,
        events: new EventTarget(),
        uploadFailed: false,
        resume: async () => {
          signal.throwIfAborted();
          if (client.isClosed || entries.get(client)?.value !== value)
            throw new MatrixSecurityError('device');
          try {
            await repository.resumeUploads(signal);
            value.uploadFailed = false;
          } catch (error) {
            value.uploadFailed = true;
            throw error;
          } finally {
            if (!signal.aborted && !client.isClosed && entries.get(client)?.value === value)
              value.events.dispatchEvent(new Event('change'));
          }
        },
      };
      const previous = entries.get(client);
      if (previous) {
        entries.delete(client);
        await previous.value.repository.close();
        signal.throwIfAborted();
      }
      const entry: Entry = { value, ready: false, started: false };
      entries.set(client, entry);
      try {
        // migrate prepares the owner key before it can read the retained source.
        await repository.migrate(source, signal);
        signal.throwIfAborted();
        if (entries.get(client) !== entry || client.isClosed)
          throw new MatrixSecurityError('device');
        entry.ready = true;
      } catch (error) {
        if (entries.get(client) === entry) entries.delete(client);
        await repository.close();
        throw error;
      }
    },
    activate: (client) => {
      const entry = entries.get(client);
      if (!entry?.ready || entry.started) return;
      entry.started = true;
      // Replays run after the verified presenter can mount; closing drains them.
      void current(client)
        .resume()
        .catch(() => {});
    },
    close: async (client) => {
      const entry = entries.get(client);
      entries.delete(client);
      await entry?.value.repository.close();
    },
  };
  return { host, current };
}
