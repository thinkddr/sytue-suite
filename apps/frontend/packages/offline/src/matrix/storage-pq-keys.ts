/* SPDX-License-Identifier: Apache-2.0 */
import type { EncryptedOfflineValues } from '../encrypted-storage.js';
import type { PqcRuntime, PqcIdentityMaterial } from '../pqc/runtime.js';
import type { MatrixStorageScope } from './types.js';
import { MatrixSecurityError } from './types.js';

export interface StoragePqBinding {
  scope: MatrixStorageScope;
  identity_version: 1 | 2;
  user_id: string;
  recovery_key_id: string;
}
interface Material extends PqcIdentityMaterial {
  secret: Uint8Array;
  pin: Uint8Array;
  secret_id: string;
  pin_id: string;
}
interface Pin {
  pin: Uint8Array;
  profile: string;
  key_id: string;
  recipient: string;
  secret_id: string;
  pin_id: string;
}
export interface StoragePqVault {
  binding: () => Promise<StoragePqBinding>;
  read: (id: string) => Promise<{ payload: Uint8Array; revision: number } | null>;
  write: (id: string, payload: Uint8Array) => Promise<void>;
  assertOpen: () => void;
}
export function samePrivateBytes(a: Uint8Array, b: Uint8Array): boolean {
  if (a.byteLength !== b.byteLength) return false;
  let mismatch = 0;
  for (let index = 0; index < a.byteLength; index++) mismatch |= a[index]! ^ b[index]!;
  return mismatch === 0;
}
function erase(material: Material | undefined): void {
  material?.secret.fill(0);
  material?.identity.fill(0);
  material?.pin.fill(0);
}

/** The Go codec owns every identity, pin, binding and SSSS name. This adapter
 * persists only through the existing encrypted device cache and SSSS vault. */
export class StoragePqKeys {
  private active: Material | undefined;
  private bindingID: string | undefined;
  private setup: Promise<void> = Promise.resolve();
  private closed = false;
  constructor(
    private readonly core: PqcRuntime,
    private readonly vault: StoragePqVault,
    private readonly cache: EncryptedOfflineValues,
  ) {}
  private check(): void {
    if (this.closed) throw new MatrixSecurityError('device');
    this.vault.assertOpen();
  }
  private async invoke<T>(
    binding: StoragePqBinding,
    operation: string,
    args: Record<string, unknown> = {},
  ): Promise<T> {
    this.check();
    const value = await this.core.invoke<T>({ operation, ...binding, ...args });
    this.check();
    return value;
  }
  private async read(id: string): Promise<Uint8Array | null> {
    this.check();
    const value = await this.vault.read(id);
    this.check();
    if (value && value.revision !== 1) {
      value.payload.fill(0);
      throw new MatrixSecurityError('recovery');
    }
    return value?.payload ?? null;
  }
  private async immutable(id: string, raw: Uint8Array): Promise<void> {
    let checked = await this.read(id);
    try {
      if (checked) {
        if (!samePrivateBytes(checked, raw)) throw new MatrixSecurityError('recovery');
        return;
      }
      await this.vault.write(id, raw);
      this.check();
      checked = await this.read(id);
      if (!checked || !samePrivateBytes(checked, raw)) throw new MatrixSecurityError('recovery');
    } finally {
      checked?.fill(0);
    }
  }
  private async fromPin(binding: StoragePqBinding, raw: Uint8Array): Promise<Material> {
    const pin = await this.invoke<Pin>(binding, 'storage.keyring.pin', { pin: raw });
    const secret = await this.read(pin.secret_id);
    try {
      if (!secret) throw new MatrixSecurityError('recovery');
      const material = await this.invoke<Material>(binding, 'storage.keyring.open', { secret });
      if (!samePrivateBytes(material.pin, pin.pin)) {
        erase(material);
        throw new MatrixSecurityError('recovery');
      }
      return material;
    } finally {
      secret?.fill(0);
      pin.pin.fill(0);
    }
  }
  private async load(binding: StoragePqBinding, create: boolean, refresh: boolean): Promise<void> {
    const bindingID = JSON.stringify(binding);
    if (this.bindingID && this.bindingID !== bindingID) throw new MatrixSecurityError('recovery');
    if (this.active && !refresh) return;
    let cached = this.active;
    if (!cached) {
      const raw = await this.cache.read('active');
      try {
        if (raw)
          cached = await this.invoke<Material>(binding, 'storage.keyring.open', { secret: raw });
      } finally {
        raw?.fill(0);
      }
    }
    const ids = await this.invoke<{ pin_id: string }>(binding, 'storage.keyring.ids', {
      key_id: '',
    });
    let rawPin = await this.read(ids.pin_id);
    try {
      if (!rawPin) {
        // A retained local identity forbids replacing a missing or rolled-back pin.
        if (cached || !create) throw new MatrixSecurityError('recovery');
        const candidate = await this.invoke<Material>(binding, 'storage.keyring.create');
        try {
          await this.immutable(candidate.secret_id, candidate.secret);
          await this.vault.write(candidate.pin_id, candidate.pin);
          this.check();
        } finally {
          erase(candidate);
        }
        rawPin = await this.read(ids.pin_id);
        if (!rawPin) throw new MatrixSecurityError('recovery');
      }
      const selected = await this.fromPin(binding, rawPin);
      if (cached && !samePrivateBytes(cached.pin, selected.pin)) {
        erase(selected);
        throw new MatrixSecurityError('recovery');
      }
      try {
        await this.cache.write('active', selected.secret);
        this.check();
      } catch (error) {
        erase(selected);
        throw error;
      }
      if (this.active !== cached) erase(cached);
      erase(this.active);
      this.active = selected;
      this.bindingID = bindingID;
    } catch (error) {
      if (cached !== this.active) erase(cached);
      throw error;
    } finally {
      rawPin?.fill(0);
    }
  }
  async prepare(): Promise<void> {
    const task = this.setup
      .catch(() => {})
      .then(async () => {
        const binding = await this.vault.binding();
        await this.load(binding, true, true);
      });
    this.setup = task;
    await task;
  }
  async key(requested = ''): Promise<PqcIdentityMaterial> {
    const task = this.setup
      .catch(() => {})
      .then(async () => {
        const binding = await this.vault.binding();
        await this.load(binding, requested === '', false);
        this.check();
        if (!requested || this.active?.key_id === requested) {
          const value = this.active;
          if (!value) throw new MatrixSecurityError('recovery');
          return {
            profile: value.profile,
            key_id: value.key_id,
            recipient: value.recipient,
            identity: value.identity.slice(),
          };
        }
        const ids = await this.invoke<{ secret_id: string }>(binding, 'storage.keyring.ids', {
          key_id: requested,
        });
        const raw = await this.read(ids.secret_id);
        let material: Material | undefined;
        try {
          if (!raw) throw new MatrixSecurityError('recovery');
          material = await this.invoke<Material>(binding, 'storage.keyring.open', { secret: raw });
          if (material.key_id !== requested) throw new MatrixSecurityError('recovery');
          return {
            profile: material.profile,
            key_id: material.key_id,
            recipient: material.recipient,
            identity: material.identity.slice(),
          };
        } finally {
          raw?.fill(0);
          erase(material);
        }
      });
    this.setup = task.then(
      () => {},
      () => {},
    );
    return task;
  }
  close(): void {
    this.closed = true;
    erase(this.active);
    this.active = undefined;
    this.cache.close();
  }
  async destroy(): Promise<void> {
    this.close();
    await this.cache.destroy();
  }
}
