// SPDX-License-Identifier: Apache-2.0
import type { APIContext } from 'astro';
import { EncryptedBackupUnavailable, validateImportBeforeUpload } from '../client/onexGuard.js';
import { readBackupBytes } from './readBackupBody.js';

type ImportClient = {
  previewNotebook: (
    volcado: Uint8Array<ArrayBuffer>,
    name?: string,
    signal?: AbortSignal,
  ) => Promise<unknown>;
  importNotebook: (
    volcado: Uint8Array<ArrayBuffer>,
    name?: string,
    signal?: AbortSignal,
  ) => Promise<unknown>;
};

type ImportDependencies = {
  conCliente: (ctx: APIContext) => Promise<{ client: ImportClient } | Response>;
  fallo: (error: unknown) => Response;
  json: (body: unknown, status?: number) => Response;
};

// Inject the authenticated client and standard BFF responses without changing
// global modules, so the route's forwarding boundary is testable in isolation.
export function createNotebookImportHandler({ conCliente, fallo, json }: ImportDependencies) {
  return async (ctx: APIContext): Promise<Response> => {
    const c = await conCliente(ctx);
    if (c instanceof Response) return c;
    // Bytes, not text: a ZIP of Markdown is binary. Only what the service will
    // read as a `.onex` goes through the encrypted-backup guard; `name` decides,
    // as in the service, whether a loose text file is Markdown.
    const name = ctx.url.searchParams.get('name') || undefined;
    let volcado: Uint8Array<ArrayBuffer>;
    try {
      volcado = await readBackupBytes(ctx.request);
      validateImportBeforeUpload(volcado, name);
    } catch (err) {
      if (err instanceof EncryptedBackupUnavailable) {
        return json({ error: 'encrypted_backup_review_required' }, 409);
      }
      return json({ error: 'invalid_backup' }, 400);
    }
    try {
      if (ctx.request.signal.aborted) return json({ error: 'request_aborted' }, 400);
      if (ctx.url.searchParams.get('preview') === '1') {
        const result = await c.client.previewNotebook(volcado, name, ctx.request.signal);
        if (ctx.request.signal.aborted) return json({ error: 'request_aborted' }, 400);
        return json(result);
      }
      const result = await c.client.importNotebook(volcado, name, ctx.request.signal);
      if (ctx.request.signal.aborted) return json({ error: 'import_result_unknown' }, 502);
      return json(result, 201);
    } catch (err) {
      // ImportNotebook can commit a notebook or part of its notes before any
      // network, response or server error. A failure is not evidence of rollback.
      if (ctx.url.searchParams.get('preview') !== '1') {
        return json({ error: 'import_result_unknown' }, 502);
      }
      return fallo(err);
    }
  };
}
