/* SPDX-License-Identifier: Apache-2.0 */
/** Buffers crossing the worker boundary are owned by that message. */
export function ownedTransfers(
  value: unknown,
  out: Transferable[] = [],
  seen = new Set<unknown>(),
): Transferable[] {
  if (!value || typeof value !== 'object' || seen.has(value)) return out;
  seen.add(value);
  if (value instanceof Uint8Array && value.buffer instanceof ArrayBuffer) {
    if (!out.includes(value.buffer)) out.push(value.buffer);
  } else for (const field of Object.values(value)) ownedTransfers(field, out, seen);
  return out;
}
export function eraseOwnedBytes(value: unknown, seen = new Set<unknown>()): void {
  if (!value || typeof value !== 'object' || seen.has(value)) return;
  seen.add(value);
  if (value instanceof Uint8Array) value.fill(0);
  else for (const field of Object.values(value)) eraseOwnedBytes(field, seen);
}
