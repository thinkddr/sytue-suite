/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { DriveClient } from './index.js';
import {
  maxCipherBlockBytes,
  encodeStorageScopeHeader,
  storageScopeHeader,
  type MatrixStorageScope,
} from '@sytue/config/matrix';
const id = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
const scope: MatrixStorageScope = { tenant: 'acme', subject: 'person', product: 'mail' };
const receipt = { id, product: 'mail', size_bytes: 3, sha256: 'a'.repeat(64) };

test('private ciphertext endpoints preserve product/opaque ID, bounded bytes and cancellation without sending descriptors', async () => {
  const requests: { url: string; init?: RequestInit }[] = [];
  const sdk = new DriveClient({
    baseUrl: 'https://drive.test',
    auth: { getAccessToken: () => 'artificial' },
    fetchImpl: async (url, init) => {
      requests.push({ url: String(url), ...(init ? { init } : {}) });
      if (String(url).endsWith('/content')) return new Response(Uint8Array.of(1, 2, 3));
      if (init?.method === 'DELETE') return new Response(null, { status: 204 });
      return Response.json(receipt);
    },
  });
  const signal = new AbortController().signal,
    input = Uint8Array.of(1, 2, 3);
  const pending = sdk.putCipherBlob(scope, id, input, signal);
  input.fill(0);
  expect(await pending).toEqual(receipt);
  expect(requests[0]!.url).toBe('https://drive.test/api/v1/cipher/blobs/mail/' + id);
  expect(requests[0]!.init?.body).toEqual(Uint8Array.of(1, 2, 3));
  expect(new Headers(requests[0]!.init?.headers).get('content-type')).toBe(
    'application/octet-stream',
  );
  expect(requests[0]!.init?.signal).toBe(signal);
  expect(await sdk.cipherBlob(scope, id)).toEqual(receipt);
  expect(await sdk.readCipherBlob(scope, id, signal)).toEqual(Uint8Array.of(1, 2, 3));
  await sdk.deleteCipherBlob(scope, id);
  expect(requests).toHaveLength(4);
  for (const request of requests)
    expect(new Headers(request.init?.headers).get(storageScopeHeader)).toBe(
      encodeStorageScopeHeader(scope),
    );
  await expect(sdk.readCipherBlob(scope, '../invalid')).rejects.toThrow('cipher_blob_invalid');
  await expect(
    sdk.putCipherBlob(scope, id, new Uint8Array(maxCipherBlockBytes + 1)),
  ).rejects.toThrow('cipher_blob_invalid');
  expect(requests).toHaveLength(4);
});

test('a foreign or mismatching ciphertext receipt is never accepted as a confirmed upload', async () => {
  for (const bad of [
    { ...receipt, id: 'A'.repeat(26) },
    { ...receipt, product: 'drive' },
    { ...receipt, size_bytes: 4 },
    { ...receipt, sha256: 'invalid' },
  ]) {
    const sdk = new DriveClient({
      baseUrl: 'https://drive.test',
      auth: { getAccessToken: () => 'artificial' },
      fetchImpl: async () => Response.json(bad),
    });
    await expect(sdk.putCipherBlob(scope, id, Uint8Array.of(1, 2, 3))).rejects.toThrow(
      'cipher_blob_invalid',
    );
  }
});

test('scope is captured before asynchronous authentication and invalid owners never reach HTTP', async () => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const original: MatrixStorageScope = { ...scope };
  let seen: RequestInit | undefined;
  const sdk = new DriveClient({
    baseUrl: 'https://drive.test',
    auth: {
      getAccessToken: async () => {
        await gate;
        return 'artificial';
      },
    },
    fetchImpl: async (_url, init) => {
      seen = init;
      return Response.json(receipt);
    },
  });
  const pending = sdk.putCipherBlob(original, id, Uint8Array.of(1, 2, 3));
  original.tenant = 'other';
  original.subject = 'other';
  original.product = 'drive';
  release();
  expect(await pending).toEqual(receipt);
  expect(new Headers(seen?.headers).get(storageScopeHeader)).toBe(encodeStorageScopeHeader(scope));
  seen = undefined;
  for (const invalid of [
    { ...scope, subject: '' },
    { ...scope, tenant: 'a'.repeat(64) },
    { ...scope, subject: '\ud800' },
    { ...scope, subject: 'a\n' },
  ]) {
    await expect(sdk.readCipherBlob(invalid, id)).rejects.toThrow('cipher_blob_invalid');
  }
  expect(seen).toBeUndefined();
});
