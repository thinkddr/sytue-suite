/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El SHA-256 incremental de hash-wasm, bajado la primera vez que se usa.
 *
 * Solo hace falta al subir, bajar o migrar un fichero, y hash-wasm es un único módulo
 * con todos sus algoritmos: importarlo de forma estática lo dejaba en el JS inicial de
 * cada app con bóveda y volvía inútil el `import()` del Argon2id de `recovery-wrap.ts`
 * (el aviso INEFFECTIVE_DYNAMIC_IMPORT del build). Todos lo piden con `import()`, así
 * que vive en su propio trozo.
 */
import type { IHasher } from 'hash-wasm';

export type { IHasher };

export async function createSHA256(): Promise<IHasher> {
  const { createSHA256: crear } = await import('hash-wasm');
  return crear();
}
