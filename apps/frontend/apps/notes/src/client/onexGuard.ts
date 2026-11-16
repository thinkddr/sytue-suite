// SPDX-License-Identifier: Apache-2.0
import { parse } from 'lossless-json';

export class EncryptedBackupUnavailable extends Error {
  constructor() {
    super('notes: encrypted backup requires local import');
    this.name = 'EncryptedBackupUnavailable';
  }
}

// Go's decoder matches struct keys case-insensitively; duplicate aliases must
// not let this check interpret a different notebook.
function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('invalid backup');
  return value as Record<string, unknown>;
}
function field(value: Record<string, unknown>, name: string): unknown {
  const keys = Object.keys(value).filter((key) => key.toLowerCase().replaceAll('ſ', 's') === name);
  if (keys.length > 1) throw new Error('ambiguous backup');
  return keys.length ? value[keys[0]!] : undefined;
}

/** An encrypted archive must remain local until it can be imported privately. */
export function validateNotebookBackupBeforeUpload(raw: string): void {
  if (raw.length > 32 * 1024 * 1024 || new Blob([raw]).size > 32 * 1024 * 1024)
    throw new Error('backup too large');
  const doc = object(
    parse(raw, undefined, {
      onDuplicateKey: () => {
        throw new Error('ambiguous backup');
      },
    }),
  );
  const book = object(field(doc, 'notebook'));
  const rawRows = field(doc, 'notas');
  if (rawRows != null && !Array.isArray(rawRows)) throw new Error('invalid backup');
  const rows = rawRows ?? [];
  const encrypted = field(book, 'encrypted');
  if (encrypted != null && typeof encrypted !== 'boolean') throw new Error('invalid backup');
  if (encrypted) throw new EncryptedBackupUnavailable();
  for (const value of rows) {
    const note = value == null ? {} : object(value);
    const cipher = field(note, 'content_cipher');
    if (cipher != null && cipher !== '') throw new EncryptedBackupUnavailable();
  }
}

/** Formats the notes service imports; mirrors `model.LeerImportable` in Go. */
export type FormatoImportable = 'zip' | 'enex' | 'markdown' | 'onex';

const MAX_IMPORT_BYTES = 32 * 1024 * 1024;
const ENEX_MARK = new TextEncoder().encode('<en-export');

function contiene(datos: Uint8Array, aguja: Uint8Array): boolean {
  fuera: for (let i = 0; i + aguja.length <= datos.length; i++) {
    for (let j = 0; j < aguja.length; j++) if (datos[i + j] !== aguja[j]) continue fuera;
    return true;
  }
  return false;
}

function utf8Valido(datos: Uint8Array): boolean {
  try {
    new TextDecoder('utf-8', { fatal: true }).decode(datos);
    return true;
  } catch {
    return false;
  }
}

/**
 * Recognises the format by content, exactly as the service does: a ZIP of
 * Markdown, an Evernote export, a loose Markdown file by its extension, and
 * otherwise a `.onex` backup. Only the `.onex` can carry encrypted notes.
 */
export function formatoImportable(datos: Uint8Array, nombre = ''): FormatoImportable {
  if (datos[0] === 0x50 && datos[1] === 0x4b && datos[2] === 0x03 && datos[3] === 0x04)
    return 'zip';
  if (contiene(datos.subarray(0, 4096), ENEX_MARK)) return 'enex';
  const base = nombre.replaceAll('\\', '/').split('/').at(-1)?.trim() ?? '';
  const punto = base.lastIndexOf('.');
  const ext = punto >= 0 ? base.slice(punto).toLowerCase() : '';
  if ((ext === '.md' || ext === '.markdown' || ext === '.txt') && utf8Valido(datos))
    return 'markdown';
  return 'onex';
}

/**
 * Applies the local backup guard to whatever the service will read as a
 * `.onex`; Markdown and Evernote imports carry no ciphertext and travel as bytes.
 */
export function validateImportBeforeUpload(datos: Uint8Array, nombre = ''): void {
  if (datos.byteLength > MAX_IMPORT_BYTES) throw new Error('backup too large');
  if (formatoImportable(datos, nombre) !== 'onex') return;
  validateNotebookBackupBeforeUpload(new TextDecoder('utf-8', { fatal: true }).decode(datos));
}
