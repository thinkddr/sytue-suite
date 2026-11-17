/* SPDX-License-Identifier: Apache-2.0 */
/** Local device cache only. Network/storage E2E continues to use maintained Matrix. */
import {
  readOfflineValue,
  mutateOfflineValue,
  listOfflineIDs,
  pageOfflineIDs,
  deleteOfflineScope,
  deleteOfflineValue,
  offlineStorageEpoch,
} from './storage.js';
const keyNamespace = 'local-crypto-key-v1',
  valueNamespace = 'local-crypto-value-v1';
const maximumBytes = 8 << 20;
interface SealedValue {
  v: 1;
  iv: Uint8Array<ArrayBuffer>;
  ciphertext: Uint8Array<ArrayBuffer>;
}
interface KeyValue {
  v: 1;
  key: CryptoKey;
}
export interface EncryptedOfflinePort {
  read: typeof readOfflineValue;
  mutate: typeof mutateOfflineValue;
  ids: (ownerId: string, namespace: string, maximum: number, prefix?: string) => Promise<string[]>;
  pageIDs: typeof pageOfflineIDs;
  deleteScope: typeof deleteOfflineScope;
  delete: typeof deleteOfflineValue;
  epoch: () => number;
}
const standard: EncryptedOfflinePort = {
  read: readOfflineValue,
  mutate: mutateOfflineValue,
  ids: listOfflineIDs,
  pageIDs: pageOfflineIDs,
  deleteScope: deleteOfflineScope,
  delete: deleteOfflineValue,
  epoch: offlineStorageEpoch,
};
export class EncryptedOfflineError extends Error {
  constructor() {
    super('encrypted_offline_unavailable');
    this.name = 'EncryptedOfflineError';
  }
}
// Earlier JSON persistence reduced a non-extractable CryptoKey to {}. Repair
// only that exact empty descriptor, and only when no ciphertext depends on it.
function emptyLostKey(value: unknown): boolean {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const record = value as Record<string, unknown>;
  const key = record.key;
  return (
    record.v === 1 &&
    Object.keys(record).length === 2 &&
    !!key &&
    typeof key === 'object' &&
    !(key instanceof CryptoKey) &&
    !Array.isArray(key) &&
    Object.keys(key).length === 0
  );
}
function keyValue(value: unknown): CryptoKey {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new EncryptedOfflineError();
  const record = value as Record<string, unknown>;
  const key = record.key;
  if (
    record.v !== 1 ||
    !(key instanceof CryptoKey) ||
    key.type !== 'secret' ||
    key.extractable ||
    key.algorithm.name !== 'AES-GCM' ||
    (key.algorithm as AesKeyAlgorithm).length !== 256 ||
    !key.usages.includes('encrypt') ||
    !key.usages.includes('decrypt')
  )
    throw new EncryptedOfflineError();
  return key;
}
function sealedValue(value: unknown): SealedValue {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new EncryptedOfflineError();
  const record = value as Record<string, unknown>;
  if (
    record.v !== 1 ||
    !(record.iv instanceof Uint8Array) ||
    record.iv.byteLength !== 12 ||
    !(record.ciphertext instanceof Uint8Array) ||
    record.ciphertext.byteLength < 16 ||
    record.ciphertext.byteLength > maximumBytes + 16
  )
    throw new EncryptedOfflineError();
  return { v: 1, iv: record.iv.slice(), ciphertext: record.ciphertext.slice() };
}
/**
 * Non-extractable device cache with authenticated account/namespace/record context.
 * Writers hold the host's exclusive device lease (SuiteMatrix already owns it).
 */
export class EncryptedOfflineValues {
  private closed = false;
  private readonly owner: string;
  private readonly encoder = new TextEncoder();
  private readonly epoch: number;
  constructor(
    private readonly ownerId: string,
    private readonly namespace: string,
    private readonly active: () => boolean = () => true,
    private readonly port: EncryptedOfflinePort = standard,
  ) {
    if (
      !ownerId ||
      !namespace ||
      this.encoder.encode(ownerId).byteLength > 16384 ||
      this.encoder.encode(namespace).byteLength > 255
    )
      throw new EncryptedOfflineError();
    this.epoch = port.epoch();
    // Packing both dimensions prevents separator collisions in the existing IDB plumbing.
    this.owner = JSON.stringify([ownerId, namespace]);
  }
  private check(epoch: number): void {
    if (this.closed || !this.active() || this.port.epoch() !== epoch || epoch !== this.epoch)
      throw new EncryptedOfflineError();
  }
  private aad(id: string): Uint8Array<ArrayBuffer> {
    if (!id || this.encoder.encode(id).byteLength > 4096) throw new EncryptedOfflineError();
    return this.encoder.encode(
      JSON.stringify(['sytue-local-encryption-v1', this.ownerId, this.namespace, id]),
    );
  }
  private async key(create: boolean, epoch: number): Promise<CryptoKey> {
    this.check(epoch);
    const stored = await this.port.read<unknown>(this.owner, keyNamespace, 'root');
    this.check(epoch);
    if (stored !== null && !emptyLostKey(stored)) return keyValue(stored);
    if (!create) throw new EncryptedOfflineError();
    // Any retained value forbids implicit key replacement. Overflow also fails
    // closed, without materializing even one encrypted body.
    const existing = await this.port.ids(this.owner, valueNamespace, 1).catch(() => {
      throw new EncryptedOfflineError();
    });
    if (existing.length > 0) throw new EncryptedOfflineError();
    this.check(epoch);
    const candidate = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, [
      'encrypt',
      'decrypt',
    ]);
    this.check(epoch);
    await this.port.mutate<KeyValue>(this.owner, keyNamespace, 'root', (previous) => {
      this.check(epoch);
      if (previous !== null && !emptyLostKey(previous)) {
        keyValue(previous);
        return previous;
      }
      return { v: 1, key: candidate };
    });
    this.check(epoch);
    const root = await this.port.read(this.owner, keyNamespace, 'root');
    this.check(epoch);
    return keyValue(root);
  }
  async read(id: string): Promise<Uint8Array<ArrayBuffer> | null> {
    const epoch = this.port.epoch(),
      aad = this.aad(id);
    this.check(epoch);
    const raw = await this.port.read<unknown>(this.owner, valueNamespace, id);
    this.check(epoch);
    if (raw === null) return null;
    const sealed = sealedValue(raw),
      key = await this.key(false, epoch);
    this.check(epoch);
    let clear: Uint8Array<ArrayBuffer> | undefined;
    try {
      clear = new Uint8Array(
        await crypto.subtle.decrypt(
          { name: 'AES-GCM', iv: sealed.iv, additionalData: aad, tagLength: 128 },
          key,
          sealed.ciphertext,
        ),
      );
      this.check(epoch);
      return clear;
    } catch {
      clear?.fill(0);
      throw new EncryptedOfflineError();
    }
  }
  async write(id: string, value: Uint8Array): Promise<void> {
    if (value.byteLength > maximumBytes) throw new EncryptedOfflineError();
    const epoch = this.port.epoch(),
      aad = this.aad(id),
      clear = value.slice();
    try {
      this.check(epoch);
      const key = await this.key(true, epoch);
      this.check(epoch);
      const iv = crypto.getRandomValues(new Uint8Array(12));
      const ciphertext = new Uint8Array(
        await crypto.subtle.encrypt(
          { name: 'AES-GCM', iv, additionalData: aad, tagLength: 128 },
          key,
          clear,
        ),
      );
      this.check(epoch);
      await this.port.mutate<SealedValue>(this.owner, valueNamespace, id, () => {
        this.check(epoch);
        return { v: 1, iv, ciphertext };
      });
      this.check(epoch);
    } finally {
      clear.fill(0);
    }
  }
  close(): void {
    this.closed = true;
  }
  async listIDs(maximum = 1000, prefix = ''): Promise<string[]> {
    if (prefix.length > 4096 || !/^[ -~]*$/.test(prefix)) throw new EncryptedOfflineError();
    const epoch = this.port.epoch();
    this.check(epoch);
    const ids = await this.port.ids(this.owner, valueNamespace, maximum, prefix);
    this.check(epoch);
    for (const id of ids) {
      this.aad(id);
      if (!id.startsWith(prefix)) throw new EncryptedOfflineError();
    }
    return ids;
  }
  /** Bounded metadata page; the cursor belongs to this exact account and prefix. */
  async listIDPage(maximum = 128, prefix = '', after = '') {
    if (
      !Number.isSafeInteger(maximum) ||
      maximum < 1 ||
      maximum > 10000 ||
      prefix.length > 4096 ||
      !/^[ -~]*$/.test(prefix) ||
      after.length > 4096 ||
      !/^[ -~]*$/.test(after) ||
      (after && !after.startsWith(prefix))
    )
      throw new EncryptedOfflineError();
    const epoch = this.port.epoch();
    this.check(epoch);
    const page = await this.port.pageIDs(this.owner, valueNamespace, maximum, prefix, after);
    this.check(epoch);
    let previous = after;
    if (page.ids.length > maximum) throw new EncryptedOfflineError();
    for (const id of page.ids) {
      this.aad(id);
      if (!id.startsWith(prefix) || (previous && id <= previous)) throw new EncryptedOfflineError();
      previous = id;
    }
    if (page.next && (page.ids.length !== maximum || page.next !== previous))
      throw new EncryptedOfflineError();
    return page;
  }
  /** Checks invalidate deletion from a closed/previous session without reading the value. */
  async delete(id: string): Promise<void> {
    const epoch = this.port.epoch();
    this.aad(id);
    this.check(epoch);
    await this.port.delete(this.owner, valueNamespace, id, () => this.check(epoch));
    this.check(epoch);
  }
  /** Key erasure first. An interrupted cleanup cannot turn retained ciphertext into plaintext. */
  async destroy(): Promise<void> {
    this.close();
    await this.port.delete(this.owner, keyNamespace, 'root');
    await this.port.deleteScope(this.owner, valueNamespace);
  }
}
