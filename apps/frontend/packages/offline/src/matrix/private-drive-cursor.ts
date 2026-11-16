/* SPDX-License-Identifier: Apache-2.0 */
/** Bounded opaque local query cursors; the domain validates the owner and fields. */
import { encodeStorageJSON, storageJSONBytes } from './storage.js';
import { json } from './private-drive-codec.js';
import { invalid } from './storage-file-codec.js';
export function driveCursor(value: string): Record<string, unknown> {
  if (value.length > 4096 || !/^[A-Za-z0-9_-]+$/.test(value)) invalid();
  const normal = value.replaceAll('-', '+').replaceAll('_', '/');
  return json(storageJSONBytes(normal + '='.repeat((4 - (normal.length % 4)) % 4), 4096));
}
export function encodeDriveCursor(value: Record<string, unknown>): string {
  return encodeStorageJSON(new TextEncoder().encode(JSON.stringify(value)))
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replace(/=+$/, '');
}
