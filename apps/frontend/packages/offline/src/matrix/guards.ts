/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Comprobaciones puras que usan también las apps sin Matrix (Drive, Photos, Mail en
 * la bóveda). Viven fuera de `client.ts` para que importarlas no arrastre
 * matrix-js-sdk al JS inicial: `client.ts` las reexporta tal cual.
 */
import type { IContent as MatrixCustomContent } from 'matrix-js-sdk';

/** Active documents must never execute on the Rooms origin after decryption. */
export function safeAttachmentType(type: string): string {
  return /^(image\/(png|jpeg|webp|gif|avif|bmp)|application\/pdf|text\/plain|audio\/[a-z0-9.+-]+|video\/[a-z0-9.+-]+)$/.test(
    type,
  )
    ? type
    : 'application/octet-stream';
}
export type MatrixCallEventType =
  'org.sytue.rooms.call_key.v1' | 'org.sytue.rooms.call_key_request.v1';
export interface MatrixCallKeyContent extends MatrixCustomContent {
  v: 1;
  call_id: string;
  generation: number;
  algorithm: 'AES-GCM';
  expires_at: number;
  key_b64?: string;
}

export function validCallContent(
  type: MatrixCallEventType,
  content: MatrixCallKeyContent,
): boolean {
  return (
    content.v === 1 &&
    typeof content.call_id === 'string' &&
    content.call_id.length > 0 &&
    content.call_id.length <= 256 &&
    Number.isSafeInteger(content.generation) &&
    content.generation >= 0 &&
    content.algorithm === 'AES-GCM' &&
    Number.isSafeInteger(content.expires_at) &&
    content.expires_at > Date.now() &&
    content.expires_at <= Date.now() + 24 * 60 * 60 * 1000 &&
    (type === 'org.sytue.rooms.call_key_request.v1'
      ? content.key_b64 === undefined
      : typeof content.key_b64 === 'string' && /^[A-Za-z0-9+/]{43}=$/.test(content.key_b64))
  );
}
