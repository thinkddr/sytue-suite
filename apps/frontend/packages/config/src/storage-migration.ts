/* SPDX-License-Identifier: Apache-2.0 */
/** Retained legacy source, not an assertion that migration or E2E is complete. */
import {
  isMatrixStorageScope,
  isStorageIdentifier,
  isStorageText,
  type MatrixStorageScope,
} from './matrix.js';
export const maxLegacySourceRows = 50;
export const maxLegacySourceBytes = 64 << 10;
export interface LegacyMigrationState {
  scope: MatrixStorageScope;
  mode: 'legacy' | 'frozen' | 'private';
  epoch?: string;
  started_at?: string;
  rows: number;
  blobs: number;
}
export interface LegacySourceCursor {
  kind: string;
  id: string;
}
export interface LegacySourceRow extends LegacySourceCursor {
  /** Exact UTF-8 JSON in standard base64. Preserve these bytes, including domain integers. */
  payload: string;
}
export interface LegacySourcePage {
  scope: MatrixStorageScope;
  epoch: string;
  rows: LegacySourceRow[];
  next?: LegacySourceCursor;
}
export class StorageMigrationError extends Error {
  constructor() {
    super('private_storage_source_invalid');
    this.name = 'StorageMigrationError';
  }
}
function invalid(): never {
  throw new StorageMigrationError();
}
function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return invalid();
  return value as Record<string, unknown>;
}
export function isMigrationEpoch(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value) &&
    value !== '00000000-0000-0000-0000-000000000000'
  );
}
export function legacySourceCursor(value: unknown): LegacySourceCursor {
  const row = object(value);
  if (!isStorageIdentifier(row.kind) || !isStorageText(row.id, 2048) || /[\0\r\n\t]/.test(row.id))
    return invalid();
  return { kind: row.kind, id: row.id };
}
function scope(value: unknown, expected: MatrixStorageScope): MatrixStorageScope {
  if (
    !isMatrixStorageScope(value) ||
    !isMatrixStorageScope(expected) ||
    value.tenant !== expected.tenant ||
    value.subject !== expected.subject ||
    value.product !== expected.product
  )
    return invalid();
  return { tenant: value.tenant, subject: value.subject, product: value.product };
}
export function decodeLegacyMigrationState(
  value: unknown,
  expected: MatrixStorageScope,
): LegacyMigrationState {
  const row = object(value),
    pinned = scope(row.scope, expected);
  if (
    !Number.isSafeInteger(row.rows) ||
    !Number.isSafeInteger(row.blobs) ||
    (row.rows as number) < 0 ||
    (row.blobs as number) < 0 ||
    (row.blobs as number) > (row.rows as number)
  )
    return invalid();
  if (row.mode === 'legacy') {
    if (
      row.epoch !== undefined ||
      row.started_at !== undefined ||
      row.rows !== 0 ||
      row.blobs !== 0
    )
      return invalid();
    return { scope: pinned, mode: 'legacy', rows: 0, blobs: 0 };
  }
  if (
    (row.mode !== 'frozen' && row.mode !== 'private') ||
    !isMigrationEpoch(row.epoch) ||
    typeof row.started_at !== 'string' ||
    !/^\d{4}-\d{2}-\d{2}T/.test(row.started_at) ||
    !Number.isFinite(Date.parse(row.started_at))
  )
    return invalid();
  return {
    scope: pinned,
    mode: row.mode,
    epoch: row.epoch,
    started_at: row.started_at,
    rows: row.rows as number,
    blobs: row.blobs as number,
  };
}
/** PostgreSQL COLLATE C and Go strings compare UTF-8 bytes, unlike JS UTF-16 ordering. */
export function compareLegacySourceCursor(
  left: LegacySourceCursor,
  right: LegacySourceCursor,
): number {
  const compare = (a: string, b: string) => {
    const encoder = new TextEncoder(),
      x = encoder.encode(a),
      y = encoder.encode(b);
    for (let i = 0; i < Math.min(x.length, y.length); i++) if (x[i] !== y[i]) return x[i]! - y[i]!;
    return x.length - y.length;
  };
  return compare(left.kind, right.kind) || compare(left.id, right.id);
}
export function legacySourceBytes(payload: string): Uint8Array<ArrayBuffer> {
  if (payload.length > 4 * Math.ceil(maxLegacySourceBytes / 3)) return invalid();
  try {
    const raw = atob(payload);
    if (raw.length < 2 || raw.length > maxLegacySourceBytes || btoa(raw) !== payload)
      return invalid();
    const bytes = Uint8Array.from(raw, (c) => c.charCodeAt(0));
    // Parse only for validation. Never re-encode these numbers through JavaScript.
    JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes));
    return bytes;
  } catch {
    return invalid();
  }
}
export function decodeLegacySourcePage(
  value: unknown,
  expected: MatrixStorageScope,
  epoch: string,
  after?: LegacySourceCursor,
): LegacySourcePage {
  const page = object(value),
    pinned = scope(page.scope, expected);
  if (
    !isMigrationEpoch(epoch) ||
    page.epoch !== epoch ||
    !Array.isArray(page.rows) ||
    page.rows.length > maxLegacySourceRows
  )
    return invalid();
  let previous = after === undefined ? undefined : legacySourceCursor(after);
  const rows = page.rows.map((value: unknown): LegacySourceRow => {
    const row = object(value),
      cursor = legacySourceCursor(row);
    if (
      (previous && compareLegacySourceCursor(previous, cursor) >= 0) ||
      typeof row.payload !== 'string'
    )
      return invalid();
    legacySourceBytes(row.payload);
    previous = cursor;
    return { ...cursor, payload: row.payload };
  });
  const result: LegacySourcePage = { scope: pinned, epoch, rows };
  if (page.next !== undefined) {
    const next = legacySourceCursor(page.next),
      last = rows.at(-1);
    if (!last || compareLegacySourceCursor(last, next) !== 0) return invalid();
    result.next = next;
  }
  return result;
}
