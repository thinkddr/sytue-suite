/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El anfitrión de `PqVaultGate` para Sytue Mail (ADR 0087, entrega 4): por cada
 * bóveda abierta, un `PrivateMailClient` sobre `openVaultMailPrivate`. La puerta
 * es la de las demás apps: volver a entrar en Sytue ID o vaciar con confirmación.
 */
import { MatrixSecurityError } from '@sytue/offline/matrix/types';
import type { PqVault } from '@sytue/offline/vault';
import { openVaultMailPrivate, type VaultMailPrivatePorts } from '@sytue/offline/vault/mail';
import { PrivateMailClient } from './privateMail.js';

interface Entry {
  job: Promise<PrivateMailClient>;
  value?: PrivateMailClient;
  detached: boolean;
}

export function createVaultMailGate(options: {
  actor: string;
  tenant?: string | undefined;
  basePath: string;
  /** Sustituible en pruebas: el montaje real necesita el núcleo Wasm. */
  open?: (vault: PqVault, signal: AbortSignal) => Promise<VaultMailPrivatePorts>;
}) {
  const open =
    options.open ??
    ((vault: PqVault, signal: AbortSignal) =>
      openVaultMailPrivate(vault, { basePath: options.basePath, signal }));
  const entries = new Map<PqVault, Entry>();
  const host = {
    prepare: async (vault: PqVault, signal: AbortSignal): Promise<void> => {
      if (
        vault.scope.subject !== options.actor ||
        (options.tenant !== undefined && vault.scope.tenant !== options.tenant) ||
        vault.scope.product !== 'mail'
      )
        throw new MatrixSecurityError('identity');
      signal.throwIfAborted();
      let entry = entries.get(vault);
      if (!entry) {
        entry = {
          job: open(vault, signal).then((ports) => new PrivateMailClient(ports)),
          detached: false,
        };
        entries.set(vault, entry);
      }
      let value: PrivateMailClient;
      try {
        value = await entry.job;
      } catch (error) {
        if (entries.get(vault) === entry) entries.delete(vault);
        throw error;
      }
      if (entry.detached || entries.get(vault) !== entry || signal.aborted || vault.isClosed) {
        await value.closeAndDrain();
        throw new MatrixSecurityError('device');
      }
      entry.value = value;
    },
    detach: (vault: PqVault): void => {
      const entry = entries.get(vault);
      if (!entry) return;
      entry.detached = true;
      entry.value?.detach();
    },
    close: async (vault: PqVault): Promise<void> => {
      const entry = entries.get(vault);
      if (!entry) return;
      entries.delete(vault);
      entry.detached = true;
      entry.value?.detach();
      let value: PrivateMailClient;
      try {
        value = entry.value ?? (await entry.job);
      } catch {
        return;
      }
      await value.closeAndDrain();
    },
  };
  const current = (vault: PqVault): PrivateMailClient => {
    const entry = entries.get(vault);
    if (!entry?.value || entry.detached || vault.isClosed) throw new MatrixSecurityError('device');
    return entry.value;
  };
  return { host, current };
}
