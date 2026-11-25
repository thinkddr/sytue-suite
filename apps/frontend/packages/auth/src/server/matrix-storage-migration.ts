/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { appDomains, suiteApex } from '@sytue/config';
import {
  encodeStorageScopeHeader,
  isMigrationEpoch,
  storageScopeHeader,
} from '@sytue/config/matrix';
import { serverEnv, type EnvLike } from '../config.js';
import type { Bff } from './bff.js';
import { crearProxyDeServicio } from './proxy.js';
import { privateStorageScope, privateStorageFailure as fail } from './matrix-storage-scope.js';
import { sessionOwnerHeader } from '@sytue/config/session-owner';
import { contextoDerivado } from './contexto-derivado.js';

export type MatrixMigrationProduct =
  | 'drive'
  | 'contacts'
  | 'calendar'
  | 'notes'
  | 'photos'
  | 'cast'
  | 'docs'
  | 'sheets'
  | 'slides'
  | 'hub'
  | 'bases'
  | 'erp';

const migrationProducts = new Set<string>([
  'drive',
  'contacts',
  'calendar',
  'notes',
  'photos',
  'cast',
  'docs',
  'sheets',
  'slides',
  'hub',
  'bases',
  'erp',
]);

function migrationDestination(product: MatrixMigrationProduct, env: EnvLike): string {
  switch (product) {
    case 'drive':
      return env.SYTUE_DRIVE_INTERNAL_URL ?? env.SYTUE_DRIVE_API_URL ?? appDomains.drive;
    case 'contacts':
      return env.SYTUE_CONTACTS_INTERNAL_URL ?? env.SYTUE_CONTACTS_BASE_URL ?? appDomains.contacts;
    case 'calendar':
      return (
        env.SYTUE_CALENDAR_INTERNAL_URL ??
        env.SYTUE_CALENDAR_API_URL ??
        env.SYTUE_CALENDAR_BASE_URL ??
        appDomains.calendar
      );
    case 'notes':
      return (
        env.SYTUE_NOTES_INTERNAL_URL ??
        env.SYTUE_NOTES_API_URL ??
        env.SYTUE_NOTES_BASE_URL ??
        appDomains.notes
      );
    case 'photos':
      return (
        env.SYTUE_PHOTOS_INTERNAL_URL ??
        env.SYTUE_PHOTOS_API_URL ??
        env.SYTUE_PHOTOS_BASE_URL ??
        appDomains.photos
      );
    case 'cast':
      return (
        env.SYTUE_CAST_INTERNAL_URL ??
        env.SYTUE_CAST_API_URL ??
        env.PUBLIC_SYTUE_CAST_BASE_URL ??
        appDomains.cast
      );
    case 'docs':
    case 'sheets':
    case 'slides':
      return (
        env.SYTUE_DOCS_INTERNAL_URL ??
        env.SYTUE_DOCS_API_URL ??
        env.PUBLIC_SYTUE_DOCS_BASE_URL ??
        `https://docs.${suiteApex}`
      );
    case 'hub':
      return env.SYTUE_HUB_INTERNAL_URL ?? env.SYTUE_HUB_BASE_URL ?? appDomains.hub;
    case 'bases':
      return (
        env.SYTUE_BASES_INTERNAL_URL ??
        env.SYTUE_BASES_API_URL ??
        env.SYTUE_BASES_BASE_URL ??
        appDomains.bases
      );
    case 'erp':
      return (
        env.SYTUE_SYERP_INTERNAL_URL ??
        env.SYTUE_SYERP_BASE_URL ??
        env.SYTUE_ERP_INTERNAL_URL ??
        appDomains.erp
      );
  }
}

function validMigrationLeaf(product: MatrixMigrationProduct, parts: readonly string[]): boolean {
  if (parts.length !== 4) return false;
  if (product === 'drive') {
    return (
      parts[1] === 'blobs' &&
      (parts[2] === 'file' || parts[2] === 'version') &&
      isMigrationEpoch(parts[3])
    );
  }
  if (product === 'contacts') {
    return (
      parts[1] === 'blobs' &&
      ['book', 'card', 'contact', 'email', 'phone', 'group', 'member'].includes(parts[2] ?? '') &&
      isMigrationEpoch(parts[3])
    );
  }
  const kind = parts[2] ?? '',
    id = parts[3] ?? '';
  return (
    parts[1] === 'source' &&
    /^[A-Za-z0-9._:-]{1,256}$/u.test(kind) &&
    id.length > 0 &&
    !/[\0\r\n\t]/u.test(id) &&
    new TextEncoder().encode(id).length <= 2048
  );
}

// The incoming Content-Length is untrusted. Bound the actual stream before any
// upstream call and require an exact match, without buffering an unbounded body.
async function contactsMigrationRequest(request: Request): Promise<Request | Response> {
  const length = request.headers.get('content-length') ?? '';
  if (
    !/^[1-9][0-9]{0,2}$/.test(length) ||
    Number(length) > 128 ||
    request.headers.get('content-type') !== 'application/json' ||
    request.headers.has('content-encoding') ||
    !request.body
  )
    return fail(400, 'private_storage_source_invalid');
  const expected = Number(length);
  const bytes = new Uint8Array(expected);
  const reader = request.body.getReader();
  let offset = 0;
  try {
    while (true) {
      request.signal.throwIfAborted();
      const { value, done } = await reader.read();
      if (done) break;
      if (value.byteLength > expected - offset) {
        void reader.cancel().catch(() => {});
        return fail(400, 'private_storage_source_invalid');
      }
      bytes.set(value, offset);
      offset += value.byteLength;
    }
    request.signal.throwIfAborted();
    if (offset !== expected) return fail(400, 'private_storage_source_invalid');
    return new Request(request.url, {
      method: request.method,
      headers: request.headers,
      body: bytes,
      signal: request.signal,
    });
  } catch {
    void reader.cancel().catch(() => {});
    return fail(400, 'private_storage_source_invalid');
  } finally {
    bytes.fill(0);
  }
}

/** Legacy bytes are streamed only to the authenticated device for this product. */
export function rutaMigracionMatrixPrivada(options: {
  bff: Bff;
  env: EnvLike;
  fetchImpl?: typeof fetch;
  product?: MatrixMigrationProduct;
}): APIRoute {
  const env = serverEnv(options.env);
  const product = options.product ?? 'drive';
  if (!migrationProducts.has(product)) throw new Error('unsupported_matrix_product');
  const strictOwner = product !== 'drive';
  // Fetch may transparently decompress a response while retaining the original
  // Content-Length. Never pass that framing to the private source reader.
  const fetchContacts = (async (input, init) => {
    const response = await (options.fetchImpl ?? fetch)(input, init);
    const encoding = response.headers.get('content-encoding');
    if (encoding !== null && encoding.trim().toLowerCase() !== 'identity') {
      void response.body?.cancel().catch(() => {});
      throw new Error('private_storage_source_encoding');
    }
    return response;
  }) as typeof fetch;
  const proxy = crearProxyDeServicio({
    destino: migrationDestination(product, env),
    bff: options.bff,
    prefijo: '/api/v1/private-storage/',
    ...(strictOwner ? { rechazarRedirecciones: true } : {}),
    ...(strictOwner
      ? { fetchImpl: fetchContacts }
      : options.fetchImpl
        ? { fetchImpl: options.fetchImpl }
        : {}),
    prepararAutenticada: (ctx, session) => {
      const path = (ctx.params.ruta ?? '').slice('migration'.length).replace(/^\//, ''),
        parts = path.split('/'),
        method = ctx.request.method,
        // «Soltar la fuente» (ADR 0086): solo POST, con el mismo cuerpo acotado. Drive
        // la suelta al vaciar su bóveda; Photos no tiene fuente propia que soltar.
        release = path === 'release' && product !== 'photos';
      if (
        path !== '' &&
        !release &&
        !(
          isMigrationEpoch(parts[0]) &&
          ((parts.length === 2 && parts[1] === 'rows') || validMigrationLeaf(product, parts))
        )
      )
        return fail(400, 'private_storage_source_invalid');
      if (release ? method !== 'POST' : method !== 'GET' && !(path === '' && method === 'POST'))
        return fail(405, 'method_not_allowed');
      const query = ctx.url.searchParams;
      if (
        query.size !== 0 &&
        !(
          parts.length === 2 &&
          parts[1] === 'rows' &&
          query.size === 1 &&
          query.has('after') &&
          new RegExp(`^[A-Za-z0-9_-]{1,${strictOwner ? 8192 : 4096}}$`).test(
            query.get('after') ?? '',
          )
        )
      )
        return fail(400, 'private_storage_source_invalid');
      // Soltar la fuente va siempre atada a la sesión comprobada (también en Drive).
      if ((strictOwner || release) && !ctx.request.headers.has(sessionOwnerHeader))
        return fail(409, 'private_storage_session_required');
      const scope = privateStorageScope(ctx.request, session, product);
      if (scope instanceof Response) return scope;
      if (method === 'POST') {
        const length = ctx.request.headers.get('content-length') ?? '';
        if (
          !/^[1-9][0-9]{0,2}$/.test(length) ||
          Number(length) > 128 ||
          ctx.request.headers.get('content-type') !== 'application/json' ||
          ctx.request.headers.has('content-encoding')
        )
          return fail(400, 'private_storage_source_invalid');
      } else if (ctx.request.body !== null) return fail(400, 'private_storage_source_invalid');
      return {
        [storageScopeHeader]: encodeStorageScopeHeader(scope),
        ...(strictOwner ? { 'accept-encoding': 'identity' } : {}),
      };
    },
  });
  return async (ctx) => {
    // Keep the backend's exact root path (no trailing slash), while the generic
    // proxy still validates the complete path before composing its destination.
    const raw = ctx.params.ruta ?? '';
    let request = ctx.request;
    if (strictOwner && request.method === 'POST') {
      const bounded = await contactsMigrationRequest(request);
      if (bounded instanceof Response) return bounded;
      request = bounded;
    }
    const response = await proxy(
      contextoDerivado(
        ctx,
        { ...ctx.params, ruta: 'migration' + (raw ? '/' + raw : '') },
        { request },
      ),
    );
    response.headers.set('cache-control', 'no-store');
    response.headers.set('vary', 'Cookie');
    response.headers.set('x-content-type-options', 'nosniff');
    return response;
  };
}
