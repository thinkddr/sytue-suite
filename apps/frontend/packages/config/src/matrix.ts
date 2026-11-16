/* SPDX-License-Identifier: Apache-2.0 */
/** Public Matrix routing contracts. No credentials, crypto implementation or client initialization. */
export interface MatrixBootstrapMetadata {
  actor: string;
  tenant: string;
  homeserver_url: string;
  sso_provider_id: string;
  matrix_user_id: string;
  controller_user_id: string;
  channels: { channel_id: string; matrix_room_id: string }[];
  encryption: 'm.megolm.v1.aes-sha2';
  draft_room_id?: string;
  draft_room_ids?: string[];
  source_url?: string;
  storage_product?: MatrixStorageProduct;
  storage_identity_version?: 0 | 1 | 2;
  storage_room_id?: string;
  storage_room_ids?: string[];
}
const storageProducts = [
  'hub',
  'mail',
  'drive',
  'docs',
  'sheets',
  'slides',
  'pdf',
  'calendar',
  'contacts',
  'notes',
  'photos',
  'rooms',
  'cast',
  'erp',
  'bases',
] as const;
export type MatrixStorageProduct = (typeof storageProducts)[number];
export interface MatrixStorageScope {
  tenant: string;
  subject: string;
  product: MatrixStorageProduct;
}
export function isStorageIdentifier(value: unknown): value is string {
  return typeof value === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(value);
}
export function isStorageProduct(value: unknown): value is MatrixStorageProduct {
  return typeof value === 'string' && (storageProducts as readonly string[]).includes(value);
}
export * from './storage-migration.js';

export function isStorageText(value: unknown, maximum: number): value is string {
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    new TextEncoder().encode(value).length > maximum
  )
    return false;
  for (const character of value) {
    const unit = character.charCodeAt(0);
    if (character.length === 1 && unit >= 0xd800 && unit <= 0xdfff) return false;
  }
  return true;
}

export function isMatrixStorageScope(value: unknown): value is MatrixStorageScope {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const scope = value as Record<string, unknown>;
  return (
    isStorageText(scope.tenant, 63) &&
    isStorageText(scope.subject, 255) &&
    isStorageProduct(scope.product) &&
    !/[\0\r\n\t]/.test(scope.tenant + scope.subject)
  );
}

/** Public identity pin, compared with authenticated context by the ciphertext gateway. */
export const storageScopeHeader = 'X-Sytue-Storage-Scope';
export function encodeStorageScopeHeader(value: MatrixStorageScope): string {
  if (!isMatrixStorageScope(value)) throw new CipherBlobError();
  const { tenant, subject, product } = value;
  const raw = new TextEncoder().encode(JSON.stringify({ tenant, subject, product }));
  return (
    'v1.' +
    btoa(String.fromCharCode(...raw))
      .replaceAll('+', '-')
      .replaceAll('/', '_')
      .replace(/=+$/, '')
  );
}

export function decodeStorageScopeHeader(value: unknown): MatrixStorageScope {
  if (typeof value !== 'string' || value.length > 4096 || !/^v1\.[A-Za-z0-9_-]+$/.test(value))
    throw new CipherBlobError();
  try {
    const encoded = value.slice(3),
      raw = atob(encoded.replaceAll('-', '+').replaceAll('_', '/'));
    if (btoa(raw).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '') !== encoded)
      throw new CipherBlobError();
    const bytes = Uint8Array.from(raw, (character) => character.charCodeAt(0));
    const scope: unknown = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes));
    if (!isMatrixStorageScope(scope) || Object.keys(scope).length !== 3)
      throw new CipherBlobError();
    return { tenant: scope.tenant, subject: scope.subject, product: scope.product };
  } catch {
    throw new CipherBlobError();
  }
}

export interface MatrixReference {
  room_id: string;
  event_id: string;
}

/** Public ciphertext gateway receipts, shared by the HTTP client and private file codec. */
export const maxCipherBlockBytes = 8 << 20;
export interface CipherBlobReceipt {
  id: string;
  product: MatrixStorageProduct;
  size_bytes: number;
  sha256: string;
}
export class CipherBlobError extends Error {
  constructor() {
    super('cipher_blob_invalid');
    this.name = 'CipherBlobError';
  }
}
export function isCipherBlobID(value: unknown): value is string {
  return typeof value === 'string' && /^[A-Z2-7]{26,64}$/.test(value);
}
export function decodeCipherBlobReceipt(
  value: unknown,
  product: MatrixStorageProduct,
  id?: string,
): CipherBlobReceipt {
  if (!value || typeof value !== 'object' || Array.isArray(value) || !isStorageProduct(product))
    throw new CipherBlobError();
  const record = value as Record<string, unknown>;
  if (
    !isCipherBlobID(record.id) ||
    (id !== undefined && record.id !== id) ||
    record.product !== product ||
    typeof record.size_bytes !== 'number' ||
    !Number.isSafeInteger(record.size_bytes) ||
    record.size_bytes < 0 ||
    record.size_bytes > maxCipherBlockBytes ||
    typeof record.sha256 !== 'string' ||
    !/^[0-9a-f]{64}$/.test(record.sha256)
  )
    throw new CipherBlobError();
  return { id: record.id, product, size_bytes: record.size_bytes, sha256: record.sha256 };
}

export type * from './storage-authority.js';

function boundedIdentifier(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= 255 &&
    !/\s/.test(value) &&
    Array.from(value).every(
      (character) => character.charCodeAt(0) >= 32 && character.charCodeAt(0) !== 127,
    )
  );
}
export function isMatrixDeviceId(value: unknown): value is string {
  return boundedIdentifier(value);
}
/** Room v12 IDs are a SHA-256 base64url digest; earlier room versions include a server name. */
export function isMatrixRoomId(value: unknown): value is string {
  return (
    boundedIdentifier(value) && (/^![A-Za-z0-9_-]{43}$/.test(value) || /^![^:]+:.+$/.test(value))
  );
}
