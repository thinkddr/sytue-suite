/* SPDX-License-Identifier: Apache-2.0 */
import { EncryptedOfflineValues, deleteOfflineValue, readOfflineValue } from '@sytue/offline';
import type { FileAttachment } from './attachments.js';

export interface LocalComposeDraft {
  from: string;
  to: string;
  cc: string;
  bcc: string;
  subject: string;
  bodyHtml: string;
  attachments: { name: string; url: string }[];
  files: FileAttachment[];
}

const namespace = 'mail.compose';
const record = 'active';
const encoder = new TextEncoder();
const decoder = new TextDecoder();

export interface LegacyLocalDraftPort {
  read: (ownerId: string, namespace: string, id: string) => Promise<unknown>;
  delete: (ownerId: string, namespace: string, id: string) => Promise<void>;
}

const legacyStorage: LegacyLocalDraftPort = {
  read: readOfflineValue,
  delete: deleteOfflineValue,
};

export class LocalComposeDraftStore {
  private readonly values: EncryptedOfflineValues;

  constructor(
    private readonly ownerId: string,
    values?: EncryptedOfflineValues,
    private readonly legacy: LegacyLocalDraftPort = legacyStorage,
  ) {
    this.values = values ?? new EncryptedOfflineValues(ownerId, namespace);
  }

  async read(): Promise<LocalComposeDraft | null> {
    const stored = await this.values.read(record);
    if (stored !== null) {
      const draft = decodeDraft(stored);
      await this.removeLegacy();
      return draft;
    }

    const legacy = await this.legacy.read(this.ownerId, namespace, record);
    if (legacy === null) return null;
    const draft = validateDraft(legacy);
    const clear = encoder.encode(JSON.stringify(draft));
    try {
      await this.values.write(record, clear);
      const verified = await this.values.read(record);
      if (verified === null || !equalBytes(clear, verified)) {
        verified?.fill(0);
        throw new Error('encrypted_offline_unavailable');
      }
      verified.fill(0);
      await this.removeLegacy();
      return draft;
    } finally {
      clear.fill(0);
    }
  }

  async write(draft: LocalComposeDraft): Promise<void> {
    const clear = encoder.encode(JSON.stringify(validateDraft(draft)));
    try {
      await this.values.write(record, clear);
    } finally {
      clear.fill(0);
    }
  }

  async delete(): Promise<void> {
    await this.values.delete(record);
    await this.removeLegacy();
  }

  private async removeLegacy(): Promise<void> {
    await this.legacy.delete(this.ownerId, namespace, record);
  }
}

function decodeDraft(bytes: Uint8Array): LocalComposeDraft {
  try {
    return validateDraft(JSON.parse(decoder.decode(bytes)) as unknown);
  } finally {
    bytes.fill(0);
  }
}

function validateDraft(value: unknown): LocalComposeDraft {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('encrypted_offline_unavailable');
  const draft = value as Record<string, unknown>;
  const strings = ['from', 'to', 'cc', 'bcc', 'subject', 'bodyHtml'] as const;
  if (
    strings.some((key) => typeof draft[key] !== 'string') ||
    !Array.isArray(draft.attachments) ||
    !Array.isArray(draft.files)
  )
    throw new Error('encrypted_offline_unavailable');
  const attachments = draft.attachments.map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item))
      throw new Error('encrypted_offline_unavailable');
    const attachment = item as Record<string, unknown>;
    if (typeof attachment.name !== 'string' || typeof attachment.url !== 'string')
      throw new Error('encrypted_offline_unavailable');
    return { name: attachment.name, url: attachment.url };
  });
  const files = draft.files.map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item))
      throw new Error('encrypted_offline_unavailable');
    const file = item as Record<string, unknown>;
    if (
      typeof file.name !== 'string' ||
      typeof file.type !== 'string' ||
      typeof file.contentBase64 !== 'string' ||
      typeof file.size !== 'number' ||
      !Number.isSafeInteger(file.size) ||
      file.size < 0 ||
      (file.contentId !== undefined && typeof file.contentId !== 'string')
    )
      throw new Error('encrypted_offline_unavailable');
    return {
      name: file.name,
      type: file.type,
      contentBase64: file.contentBase64,
      size: file.size,
      ...(typeof file.contentId === 'string' ? { contentId: file.contentId } : {}),
    };
  });
  return {
    from: draft.from as string,
    to: draft.to as string,
    cc: draft.cc as string,
    bcc: draft.bcc as string,
    subject: draft.subject as string,
    bodyHtml: draft.bodyHtml as string,
    attachments,
    files,
  };
}

function equalBytes(left: Uint8Array, right: Uint8Array): boolean {
  if (left.byteLength !== right.byteLength) return false;
  let difference = 0;
  for (let index = 0; index < left.byteLength; index++) difference |= left[index]! ^ right[index]!;
  return difference === 0;
}
