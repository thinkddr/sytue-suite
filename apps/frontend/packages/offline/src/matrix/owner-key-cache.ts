/* SPDX-License-Identifier: Apache-2.0 */
import type { SecretStorageKeyDescription } from 'matrix-js-sdk/lib/secret-storage';
import { decodeRecoveryKey } from 'matrix-js-sdk/lib/crypto-api/recovery-key';
import type { EncryptedOfflineValues } from '../encrypted-storage.js';
import { hasSecretStorageCheck, ownerKeyID } from './owner-proof.js';
import { MatrixSecurityError } from './types.js';

type OwnerKey = [string, Uint8Array<ArrayBuffer>];
type Descriptions = Record<string, SecretStorageKeyDescription>;
/** Authorized device cache. The existing Matrix SSSS check remains authoritative. */
export class MatrixOwnerKeyCache {
  private getting: Promise<OwnerKey> | undefined;
  private pending = Promise.resolve();
  private failure: Error | undefined;
  private closed = false;
  constructor(
    private readonly values: Pick<EncryptedOfflineValues, 'read' | 'write' | 'close' | 'destroy'>,
    private readonly keys: Map<string, Uint8Array<ArrayBuffer>>,
    private readonly checkKey: (
      key: Uint8Array<ArrayBuffer>,
      description: SecretStorageKeyDescription,
    ) => Promise<boolean>,
    private readonly readRecoveryKey: (keyIDs?: readonly string[]) => Promise<string>,
    private readonly active: () => boolean,
  ) {}
  private check(): void {
    if (this.closed || !this.active()) throw new MatrixSecurityError('device');
  }
  private async stored(): Promise<OwnerKey | null> {
    this.check();
    const raw = await this.values.read('owner');
    if (raw === null) {
      this.check();
      return null;
    }
    try {
      this.check();
      if (raw[0] !== 1 || raw[1] === 0 || raw.byteLength !== 2 + raw[1]! + 32)
        throw new MatrixSecurityError('recovery');
      const id = new TextDecoder('utf-8', { fatal: true }).decode(raw.subarray(2, 2 + raw[1]!));
      if (!ownerKeyID(id)) throw new MatrixSecurityError('recovery');
      return [id, raw.slice(2 + raw[1]!)];
    } finally {
      raw.fill(0);
    }
  }
  private retain(id: string, key: Uint8Array<ArrayBuffer>): OwnerKey {
    this.check();
    const previous = this.keys.get(id);
    if (previous) {
      if (previous.length !== key.length || previous.some((value, index) => value !== key[index]))
        throw new MatrixSecurityError('recovery');
      return [id, previous];
    }
    const retained = key.slice();
    this.keys.set(id, retained);
    return [id, retained];
  }
  private async save(
    id: string,
    description: SecretStorageKeyDescription,
    key: Uint8Array<ArrayBuffer>,
  ): Promise<void> {
    this.check();
    if (
      !ownerKeyID(id) ||
      key.byteLength !== 32 ||
      !hasSecretStorageCheck(description) ||
      !(await this.checkKey(key, description))
    )
      throw new MatrixSecurityError('recovery');
    this.check();
    const previous = await this.stored();
    if (previous) {
      try {
        // Rotation requires migrating historical attestations first, in web and native alike.
        let changed = previous[0] === id ? 0 : 1;
        for (let index = 0; index < 32; index++) changed |= previous[1][index]! ^ key[index]!;
        if (changed !== 0) throw new MatrixSecurityError('recovery');
      } finally {
        previous[1].fill(0);
      }
    } else {
      const name = new TextEncoder().encode(id);
      const raw = new Uint8Array(2 + name.byteLength + 32);
      raw[0] = 1;
      raw[1] = name.byteLength;
      raw.set(name, 2);
      raw.set(key, 2 + name.byteLength);
      try {
        await this.values.write('owner', raw);
      } finally {
        raw.fill(0);
      }
    }
    this.retain(id, key);
  }
  /** The SDK callback is synchronous; retain its copied key until the queued write finishes. */
  remember(
    id: string,
    description: SecretStorageKeyDescription,
    value: Uint8Array<ArrayBuffer>,
  ): void {
    this.check();
    const key = value.slice();
    const info = { ...description };
    this.pending = this.pending
      .then(async () => {
        if (this.failure !== undefined) throw this.failure;
        await this.save(id, info, key);
      })
      .catch((error: unknown) => {
        this.failure = error instanceof Error ? error : new MatrixSecurityError('recovery');
      })
      .finally(() => {
        key.fill(0);
      });
  }
  async flush(): Promise<void> {
    await this.pending;
    this.check();
    if (this.failure !== undefined) throw this.failure;
  }
  async get(available: Descriptions): Promise<OwnerKey> {
    this.check();
    available = Object.fromEntries(
      Object.entries(available).map(([id, description]) => [id, { ...description }]),
    );
    if (this.getting) {
      const shared = await this.getting;
      await this.checkAvailable(shared, available);
      return shared;
    }
    const pending = this.getOne(available);
    this.getting = pending;
    const clear = () => {
      if (this.getting === pending) this.getting = undefined;
    };
    void pending.then(clear, clear);
    return pending;
  }
  private async checkAvailable([id, key]: OwnerKey, available: Descriptions): Promise<void> {
    this.check();
    const description = available[id];
    if (
      !description ||
      !hasSecretStorageCheck(description) ||
      !(await this.checkKey(key, description))
    )
      throw new MatrixSecurityError('recovery');
    this.check();
  }
  private async getOne(available: Descriptions): Promise<OwnerKey> {
    await this.flush();
    const saved = await this.stored();
    if (saved) {
      try {
        await this.checkAvailable(saved, available);
        return this.retain(saved[0], saved[1]);
      } finally {
        saved[1].fill(0);
      }
    }
    if (
      !Object.entries(available).some(
        ([id, description]) => ownerKeyID(id) && hasSecretStorageCheck(description),
      )
    )
      throw new MatrixSecurityError('recovery');
    const encoded = await this.readRecoveryKey(Object.keys(available));
    this.check();
    let key: Uint8Array<ArrayBuffer>;
    try {
      key = decodeRecoveryKey(encoded.trim());
    } catch {
      throw new MatrixSecurityError('recovery');
    }
    try {
      for (const [id, description] of Object.entries(available)) {
        if (
          !ownerKeyID(id) ||
          !hasSecretStorageCheck(description) ||
          !(await this.checkKey(key, description))
        )
          continue;
        this.check();
        this.remember(id, description, key);
        await this.flush();
        const retained = this.keys.get(id);
        if (!retained) throw new MatrixSecurityError('recovery');
        return [id, retained];
      }
      throw new MatrixSecurityError('recovery');
    } finally {
      key.fill(0);
    }
  }
  close(): void {
    this.closed = true;
    this.values.close();
    for (const key of this.keys.values()) key.fill(0);
    this.keys.clear();
  }
  async destroy(): Promise<void> {
    this.close();
    // Wait for in-flight IDB commits before deleting; pending UI replies are blocked by active().
    await this.pending;
    await this.values.destroy();
  }
}
