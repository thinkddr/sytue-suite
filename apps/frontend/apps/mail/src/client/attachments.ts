/* SPDX-License-Identifier: Apache-2.0 */
import type { OutgoingAttachmentInput } from './mailApi.js';

export interface FileAttachment extends OutgoingAttachmentInput {
  size: number;
}

/** Shared by the native mailbox composer and personal provider accounts. */
export async function fileToAttachment(file: File): Promise<FileAttachment> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let bin = '';
  for (const byte of bytes) bin += String.fromCharCode(byte);
  return {
    name: file.name,
    type: file.type || 'application/octet-stream',
    contentBase64: btoa(bin),
    size: file.size,
  };
}
