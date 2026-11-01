/* SPDX-License-Identifier: Apache-2.0 */
import {
  Attachment as MatrixAttachment,
  EncryptedAttachment,
  initAsync,
} from '@matrix-org/matrix-sdk-crypto-wasm';
import {
  isStorageProduct,
  MatrixSecurityError,
  type MatrixStorageProduct,
  type MatrixStorageScope,
} from './types.js';
import { StorageRecordError, storageIdentifier, storageScope } from './storage.js';

/** Maximum plaintext/ciphertext size of a single authenticated media block (8 MiB). */
export const maxCipherBlockBytes = 8 << 20;

const base32Alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

export interface BlobReceipt {
  id: string;
  product: MatrixStorageProduct;
  size_bytes: number;
  sha256: string;
}

export interface StorageMatrixFile {
  key: JsonWebKey;
  iv: string;
  hashes: { sha256: string };
  v: 'v2';
}

/**
 * PRIVATE manifest descriptor. `matrix_file` carries the AES-256-CTR key and must
 * live only inside encrypted Matrix records or the sealed local offline store.
 */
export interface StorageBlock {
  scope: MatrixStorageScope;
  object_id: string;
  revision_id: string;
  part: number;
  blob: BlobReceipt;
  matrix_file: StorageMatrixFile;
}

/** Gateway to the suite ciphertext blob endpoint (`/api/v1/cipher/blobs/{product}/{id}`). */
export interface CipherBlobGateway {
  put: (
    product: MatrixStorageProduct,
    id: string,
    ciphertext: Uint8Array<ArrayBuffer>,
    signal?: AbortSignal,
  ) => Promise<BlobReceipt>;
  read: (
    product: MatrixStorageProduct,
    id: string,
    signal?: AbortSignal,
  ) => Promise<Uint8Array<ArrayBuffer>>;
  get?: (product: MatrixStorageProduct, id: string, signal?: AbortSignal) => Promise<BlobReceipt>;
  delete?: (product: MatrixStorageProduct, id: string, signal?: AbortSignal) => Promise<void>;
}

export class StorageSourceChangedError extends Error {
  constructor() {
    super('storage_source_changed');
    this.name = 'StorageSourceChangedError';
  }
}

export function isValidBlobId(id: unknown): id is string {
  return typeof id === 'string' && /^[A-Z2-7]{26,64}$/.test(id);
}

/** Generates a 128-bit RFC 4648 uppercase Base32 identifier matching Go's `crypto/rand.Text()`. */
export function createBlobId(): string {
  const random = crypto.getRandomValues(new Uint8Array(16));
  let bits = 0;
  let value = 0;
  let output = '';
  for (const byte of random) {
    value = (value << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      output += base32Alphabet[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) {
    output += base32Alphabet[(value << (5 - bits)) & 31];
  }
  random.fill(0);
  return output;
}

export function isValidSha256Hex(value: unknown): value is string {
  return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value);
}

export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes as BufferSource));
  let hex = '';
  for (const byte of digest) hex += byte.toString(16).padStart(2, '0');
  return hex;
}

function decodeUnpaddedBase64(
  value: unknown,
  expectedBytes: number,
  urlSafe = false,
): Uint8Array<ArrayBuffer> {
  if (typeof value !== 'string' || value.length === 0 || value.includes('=')) {
    throw new StorageRecordError();
  }
  const pattern = urlSafe ? /^[A-Za-z0-9_-]+$/ : /^[A-Za-z0-9+/]+$/;
  if (!pattern.test(value) || value.length % 4 === 1) {
    throw new StorageRecordError();
  }
  const standard = urlSafe ? value.replace(/-/g, '+').replace(/_/g, '/') : value;
  const padded = standard + '='.repeat((4 - (standard.length % 4)) % 4);
  let binary: string;
  try {
    binary = atob(padded);
  } catch {
    throw new StorageRecordError();
  }
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  if (bytes.byteLength !== expectedBytes) {
    bytes.fill(0);
    throw new StorageRecordError();
  }
  let roundtrip = '';
  for (const byte of bytes) roundtrip += String.fromCharCode(byte);
  const reencoded = btoa(roundtrip).replace(/=+$/, '');
  const expected = urlSafe ? reencoded.replace(/\+/g, '-').replace(/\//g, '_') : reencoded;
  if (expected !== value) {
    bytes.fill(0);
    throw new StorageRecordError();
  }
  return bytes;
}

export function validateBlobReceipt(
  value: unknown,
  expectedProduct: MatrixStorageProduct,
  expectedId?: string,
): BlobReceipt {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new StorageRecordError();
  const receipt = value as Record<string, unknown>;
  const id = receipt.id;
  if (
    !isStorageProduct(expectedProduct) ||
    receipt.product !== expectedProduct ||
    !isValidBlobId(id) ||
    (expectedId !== undefined && id !== expectedId) ||
    typeof receipt.size_bytes !== 'number' ||
    !Number.isSafeInteger(receipt.size_bytes) ||
    receipt.size_bytes < 0 ||
    receipt.size_bytes > maxCipherBlockBytes ||
    !isValidSha256Hex(receipt.sha256)
  ) {
    throw new StorageRecordError();
  }
  return {
    id,
    product: expectedProduct,
    size_bytes: receipt.size_bytes,
    sha256: receipt.sha256,
  };
}

export function validateMatrixFileDescriptor(
  value: unknown,
  expectedCipherSha256Hex?: string,
): StorageMatrixFile {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new StorageRecordError();
  const file = value as Record<string, unknown>;
  if (file.v !== 'v2' || !file.key || typeof file.key !== 'object' || Array.isArray(file.key)) {
    throw new StorageRecordError();
  }
  const key = file.key as Record<string, unknown>;
  const ops = key.key_ops;
  if (
    key.kty !== 'oct' ||
    key.alg !== 'A256CTR' ||
    key.ext !== true ||
    !Array.isArray(ops) ||
    !ops.includes('encrypt') ||
    !ops.includes('decrypt') ||
    ops.some((op) => typeof op !== 'string')
  ) {
    throw new StorageRecordError();
  }
  const keyBytes = decodeUnpaddedBase64(key.k, 32, true);
  keyBytes.fill(0);
  const ivBytes = decodeUnpaddedBase64(file.iv, 16, false);
  ivBytes.fill(0);
  if (!file.hashes || typeof file.hashes !== 'object' || Array.isArray(file.hashes)) {
    throw new StorageRecordError();
  }
  const hashes = file.hashes as Record<string, unknown>;
  const shaBytes = decodeUnpaddedBase64(hashes.sha256, 32, false);
  let shaHex = '';
  for (const byte of shaBytes) shaHex += byte.toString(16).padStart(2, '0');
  if (expectedCipherSha256Hex !== undefined && shaHex !== expectedCipherSha256Hex) {
    throw new StorageRecordError();
  }
  return {
    key: {
      kty: 'oct',
      alg: 'A256CTR',
      ext: true,
      key_ops: ['encrypt', 'decrypt'],
      k: key.k as string,
    },
    iv: file.iv as string,
    hashes: { sha256: hashes.sha256 as string },
    v: 'v2',
  };
}

export function validateStorageBlock(
  value: unknown,
  expectedScope: MatrixStorageScope,
  expectedObject: string,
  expectedRevision: string,
  expectedPart: number,
): StorageBlock {
  const scope = storageScope(expectedScope);
  if (
    !storageIdentifier(expectedObject) ||
    !storageIdentifier(expectedRevision) ||
    !Number.isSafeInteger(expectedPart) ||
    expectedPart < 0 ||
    !value ||
    typeof value !== 'object' ||
    Array.isArray(value)
  ) {
    throw new StorageRecordError();
  }
  const raw = value as Record<string, unknown>;
  const blockScope = storageScope(raw.scope);
  if (
    blockScope.tenant !== scope.tenant ||
    blockScope.subject !== scope.subject ||
    blockScope.product !== scope.product ||
    raw.object_id !== expectedObject ||
    raw.revision_id !== expectedRevision ||
    raw.part !== expectedPart
  ) {
    throw new StorageRecordError();
  }
  const blob = validateBlobReceipt(raw.blob, scope.product);
  const matrixFile = validateMatrixFileDescriptor(raw.matrix_file, blob.sha256);
  return {
    scope,
    object_id: expectedObject,
    revision_id: expectedRevision,
    part: expectedPart,
    blob,
    matrix_file: matrixFile,
  };
}

/**
 * Encrypts one bounded block using the maintained Matrix Rust/WASM attachment
 * codec. Copies and clears temporary plaintext if requested by the caller.
 */
export async function prepareStorageBlock(
  scopeInput: MatrixStorageScope,
  object: string,
  revision: string,
  part: number,
  plaintext: Uint8Array,
): Promise<{
  block: StorageBlock;
  ciphertext: Uint8Array<ArrayBuffer>;
  plainSha256: string;
}> {
  const scope = storageScope(scopeInput);
  if (
    !storageIdentifier(object) ||
    !storageIdentifier(revision) ||
    !Number.isSafeInteger(part) ||
    part < 0 ||
    plaintext.byteLength > maxCipherBlockBytes
  ) {
    throw new StorageRecordError();
  }
  await initAsync();
  const plainCopy = new Uint8Array(plaintext.byteLength);
  plainCopy.set(plaintext);
  const plainSha256 = await sha256Hex(plainCopy);
  const encrypted = MatrixAttachment.encrypt(plainCopy);
  plainCopy.fill(0);
  try {
    const infoRaw = encrypted.mediaEncryptionInfo;
    if (!infoRaw) throw new StorageRecordError();
    const cipherRaw = encrypted.encryptedData;
    const ciphertext = new Uint8Array(cipherRaw.byteLength);
    ciphertext.set(cipherRaw);
    cipherRaw.fill(0);
    const cipherSha256 = await sha256Hex(ciphertext);
    const matrixFile = validateMatrixFileDescriptor(JSON.parse(infoRaw), cipherSha256);
    const blob: BlobReceipt = {
      id: createBlobId(),
      product: scope.product,
      size_bytes: ciphertext.byteLength,
      sha256: cipherSha256,
    };
    const block = validateStorageBlock(
      {
        scope,
        object_id: object,
        revision_id: revision,
        part,
        blob,
        matrix_file: matrixFile,
      },
      scope,
      object,
      revision,
      part,
    );
    return { block, ciphertext, plainSha256 };
  } finally {
    encrypted.free();
  }
}

/**
 * Re-creates deterministic Matrix v2 AES-256-CTR ciphertext ONLY when retrying
 * a journaled pending block whose `plain_sha256` matches the immutable source
 * byte-for-byte. Changed input fails with `StorageSourceChangedError` without
 * ever encrypting new data under a retained key/IV.
 */
export async function restorePendingStorageBlock(
  scopeInput: MatrixStorageScope,
  blockInput: StorageBlock,
  expectedPlainSha256: string,
  plaintext: Uint8Array,
): Promise<Uint8Array<ArrayBuffer>> {
  const block = validateStorageBlock(
    blockInput,
    scopeInput,
    blockInput.object_id,
    blockInput.revision_id,
    blockInput.part,
  );
  if (!isValidSha256Hex(expectedPlainSha256) || plaintext.byteLength !== block.blob.size_bytes) {
    throw plaintext.byteLength !== block.blob.size_bytes
      ? new StorageSourceChangedError()
      : new StorageRecordError();
  }
  const actualPlainSha = await sha256Hex(plaintext);
  if (actualPlainSha !== expectedPlainSha256) {
    throw new StorageSourceChangedError();
  }
  const iv = decodeUnpaddedBase64(block.matrix_file.iv, 16, false);
  const cryptoKey = await crypto.subtle.importKey(
    'jwk',
    block.matrix_file.key,
    { name: 'AES-CTR' },
    false,
    ['encrypt'],
  );
  try {
    const encrypted = new Uint8Array(
      await crypto.subtle.encrypt(
        { name: 'AES-CTR', counter: iv, length: 64 },
        cryptoKey,
        plaintext as BufferSource,
      ),
    );
    if ((await sha256Hex(encrypted)) !== block.blob.sha256) {
      encrypted.fill(0);
      throw new MatrixSecurityError('identity');
    }
    return encrypted;
  } finally {
    iv.fill(0);
  }
}

export async function putPreparedStorageBlock(
  gateway: CipherBlobGateway,
  block: StorageBlock,
  ciphertext: Uint8Array<ArrayBuffer>,
  signal?: AbortSignal,
): Promise<void> {
  signal?.throwIfAborted();
  if (
    ciphertext.byteLength !== block.blob.size_bytes ||
    (await sha256Hex(ciphertext)) !== block.blob.sha256
  ) {
    throw new StorageRecordError();
  }
  const copy = new Uint8Array(ciphertext.byteLength);
  copy.set(ciphertext);
  try {
    const rawReceipt = await gateway.put(block.scope.product, block.blob.id, copy, signal);
    const receipt = validateBlobReceipt(rawReceipt, block.scope.product, block.blob.id);
    if (receipt.size_bytes !== block.blob.size_bytes || receipt.sha256 !== block.blob.sha256) {
      throw new StorageRecordError();
    }
  } finally {
    copy.fill(0);
  }
}

export async function uploadStorageBlock(
  gateway: CipherBlobGateway,
  scope: MatrixStorageScope,
  object: string,
  revision: string,
  part: number,
  plaintext: Uint8Array,
  signal?: AbortSignal,
): Promise<StorageBlock> {
  signal?.throwIfAborted();
  const { block, ciphertext } = await prepareStorageBlock(scope, object, revision, part, plaintext);
  try {
    await putPreparedStorageBlock(gateway, block, ciphertext, signal);
    return block;
  } finally {
    ciphertext.fill(0);
  }
}

/**
 * Authenticates the full ciphertext block against the private Matrix v2 SHA-256
 * descriptor before returning plaintext. Rejects scope, object, revision, part
 * or length substitution without exposing unauthenticated bytes.
 */
export async function downloadStorageBlock(
  gateway: CipherBlobGateway,
  scope: MatrixStorageScope,
  blockInput: StorageBlock,
  object: string,
  revision: string,
  part: number,
  signal?: AbortSignal,
): Promise<Uint8Array<ArrayBuffer>> {
  signal?.throwIfAborted();
  const block = validateStorageBlock(blockInput, scope, object, revision, part);
  const ciphertext = await gateway.read(block.scope.product, block.blob.id, signal);
  signal?.throwIfAborted();
  if (ciphertext.byteLength !== block.blob.size_bytes) {
    ciphertext.fill(0);
    throw new MatrixSecurityError('identity');
  }
  await initAsync();
  const attachment = new EncryptedAttachment(ciphertext, JSON.stringify(block.matrix_file));
  try {
    const decrypted = MatrixAttachment.decrypt(attachment);
    const plain = new Uint8Array(decrypted.byteLength);
    plain.set(decrypted);
    decrypted.fill(0);
    return plain;
  } catch {
    throw new MatrixSecurityError('unverified');
  } finally {
    ciphertext.fill(0);
    attachment.free();
  }
}
