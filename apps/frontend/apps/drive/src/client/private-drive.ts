/* SPDX-License-Identifier: Apache-2.0 */
import type { DriveClient } from '@sytue/api-clients';
import {
  MatrixSecurityError,
  PrivateDriveRepository,
  type StorageRepository,
} from '@sytue/offline/matrix';
import type { PqVault } from '@sytue/offline/vault';

/** One private presenter per opened post-quantum vault; never rebound to another owner. */
export interface PrivateDriveSession {
  readonly client: { readonly isClosed: boolean };
  readonly vault?: PqVault;
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

export interface PrivateDriveVaultHost {
  prepare: (vault: PqVault, signal: AbortSignal) => Promise<void>;
  activate: (vault: PqVault) => void;
  close: (vault: PqVault) => Promise<void>;
}

export function createPrivateDriveGate(options: {
  actor: string;
  tenant?: string | undefined;
  /** Lazy: SSR never touches window, fetch or the browser store. */
  source: () => DriveClient;
}) {
  const owner = Object.freeze({ actor: options.actor, tenant: options.tenant });
  const newSource = options.source;
  const entries = new Map<PqVault, Entry>();

  const current = (vault: PqVault): PrivateDriveSession => {
    const entry = entries.get(vault);
    if (!entry?.ready || vault.isClosed || entry.value.signal.aborted)
      throw new MatrixSecurityError('device');
    return entry.value;
  };

  const host: PrivateDriveVaultHost = {
    prepare: async (vault, signal) => {
      const scope = vault.scope;
      if (
        scope.subject !== owner.actor ||
        (owner.tenant !== undefined && scope.tenant !== owner.tenant) ||
        scope.product !== 'drive'
      )
        throw new MatrixSecurityError('identity');
      signal.throwIfAborted();
      const source = newSource();
      const boundScope = Object.freeze({
        tenant: scope.tenant,
        subject: owner.actor,
        product: 'drive' as const,
      });
      const storage = vault.storage({
        read: (id, child) => source.readCipherBlob(boundScope, id, child),
        write: (id, bytes, child) => source.putCipherBlob(boundScope, id, bytes, child),
      });
      if (
        storage.scope.subject !== owner.actor ||
        storage.scope.tenant !== scope.tenant ||
        storage.scope.product !== 'drive'
      )
        throw new MatrixSecurityError('identity');
      const repository = new PrivateDriveRepository(storage);
      const value: PrivateDriveSession = {
        client: vault,
        vault,
        storage,
        repository,
        signal,
        events: new EventTarget(),
        uploadFailed: false,
        resume: async () => {
          signal.throwIfAborted();
          if (vault.isClosed || entries.get(vault)?.value !== value)
            throw new MatrixSecurityError('device');
          try {
            await repository.resumeUploads(signal);
            value.uploadFailed = false;
          } catch (error) {
            value.uploadFailed = true;
            throw error;
          } finally {
            if (!signal.aborted && !vault.isClosed && entries.get(vault)?.value === value)
              value.events.dispatchEvent(new Event('change'));
          }
        },
      };
      const previous = entries.get(vault);
      if (previous) {
        entries.delete(vault);
        await previous.value.repository.close();
        signal.throwIfAborted();
      }
      const entry: Entry = { value, ready: false, started: false };
      entries.set(vault, entry);
      try {
        // migrate prepares the vault before it can read the retained source.
        await repository.migrate(source, signal);
        signal.throwIfAborted();
        if (entries.get(vault) !== entry || vault.isClosed) throw new MatrixSecurityError('device');
        entry.ready = true;
      } catch (error) {
        if (entries.get(vault) === entry) entries.delete(vault);
        await repository.close();
        throw error;
      }
    },
    activate: (vault) => {
      const entry = entries.get(vault);
      if (!entry?.ready || entry.started) return;
      entry.started = true;
      // Replays run after the verified presenter can mount; closing drains them.
      void current(vault)
        .resume()
        .catch(() => {});
    },
    close: async (vault) => {
      const entry = entries.get(vault);
      entries.delete(vault);
      await entry?.value.repository.close();
    },
  };

  return { host, current };
}
