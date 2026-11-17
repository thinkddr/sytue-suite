/* SPDX-License-Identifier: Apache-2.0 */
const encoder = new TextEncoder(),
  decoder = new TextDecoder('utf-8', { fatal: true });
/** Framing only; domain documents stay owned raw bytes, not parsed JSON. */
export function decodeHostObject(raw: Uint8Array, reject: () => never): Record<string, unknown> {
  const value: unknown = JSON.parse(decoder.decode(raw));
  if (!value || typeof value !== 'object' || Array.isArray(value)) return reject();
  return value as Record<string, unknown>;
}
export function encodeHostJSON(value: unknown): Uint8Array<ArrayBuffer> {
  return encoder.encode(JSON.stringify(value));
}
