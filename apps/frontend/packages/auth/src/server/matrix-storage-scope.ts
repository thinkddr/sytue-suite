/* SPDX-License-Identifier: Apache-2.0 */
import {
  decodeStorageScopeHeader,
  storageScopeHeader,
  type MatrixStorageProduct,
  type MatrixStorageScope,
} from '@sytue/config/matrix';
import type { SytueSession } from '../types.js';
import { vieneDeOtroSitio } from './guard.js';
export function privateStorageFailure(status: number, error: string): Response {
  return Response.json(
    { error },
    { status, headers: { 'cache-control': 'no-store', vary: 'Cookie' } },
  );
}
/** Check the exact session used by the common proxy before any upstream effects. */
export function privateStorageScope(
  request: Request,
  session: SytueSession,
  product: MatrixStorageProduct,
): MatrixStorageScope | Response {
  const origin = request.headers.get('origin');
  if (vieneDeOtroSitio(request) || (origin !== null && origin !== new URL(request.url).origin))
    return privateStorageFailure(403, 'cross_site');
  let scope;
  try {
    scope = decodeStorageScopeHeader(request.headers.get(storageScopeHeader));
  } catch {
    return privateStorageFailure(400, 'cipher_scope_invalid');
  }
  if (
    scope.product !== product ||
    scope.tenant !== session.user.tenant ||
    scope.subject !== session.user.id
  )
    return privateStorageFailure(409, 'cipher_scope_changed');
  return scope;
}

// Shared framing guard only. Domain JSON, signatures, epochs and MIME remain in
// their original Go parsers. SMTP may stream an unknown Content-Length.
export function privateStorageBody(
  request: Request,
  maximum: number,
  contentType = 'application/json',
  streamed = false,
): Response | null {
  const length = request.headers.get('content-length');
  if (
    request.headers.get('content-type') !== contentType ||
    request.headers.has('content-encoding') ||
    (length === null
      ? !streamed
      : !/^[1-9][0-9]*$/u.test(length) ||
        !Number.isSafeInteger(Number(length)) ||
        Number(length) > maximum)
  )
    return privateStorageFailure(400, 'private_storage_body_invalid');
  return null;
}

/** Read bounded JSON framing as bytes. The original Go endpoint still parses
 * signatures and domain values; a claimed Content-Length never grants a limit. */
export async function readPrivateStorageBody(
  request: Request,
  maximum: number,
): Promise<Uint8Array<ArrayBuffer> | Response> {
  if (!Number.isSafeInteger(maximum) || maximum <= 0 || !request.body)
    return privateStorageFailure(400, 'private_storage_body_invalid');
  const reader = request.body.getReader(),
    chunks: Uint8Array[] = [];
  let total = 0,
    pending: Uint8Array | undefined,
    body: Uint8Array<ArrayBuffer> | undefined,
    transferred = false;
  const abort = () => {
    void reader.cancel().catch(() => {});
  };
  request.signal.addEventListener('abort', abort, { once: true });
  try {
    request.signal.throwIfAborted();
    for (;;) {
      const item = await reader.read();
      pending = item.done ? undefined : item.value;
      request.signal.throwIfAborted();
      if (item.done) break;
      total += item.value.byteLength;
      if (total > maximum) {
        await reader.cancel().catch(() => {});
        return privateStorageFailure(400, 'private_storage_body_invalid');
      }
      chunks.push(item.value);
      pending = undefined;
    }
    if (total === 0 || Number(request.headers.get('content-length')) !== total)
      return privateStorageFailure(400, 'private_storage_body_invalid');
    body = new Uint8Array(total);
    let offset = 0;
    for (const part of chunks) {
      body.set(part, offset);
      offset += part.byteLength;
    }
    transferred = true;
    return body;
  } catch {
    await reader.cancel().catch(() => {});
    return privateStorageFailure(400, 'private_storage_body_invalid');
  } finally {
    pending?.fill(0);
    for (const part of chunks) part.fill(0);
    if (!transferred) body?.fill(0);
    request.signal.removeEventListener('abort', abort);
    reader.releaseLock();
  }
}
