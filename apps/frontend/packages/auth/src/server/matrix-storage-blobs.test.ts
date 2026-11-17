/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { readFile } from 'node:fs/promises';
import type { APIContext } from 'astro';
import {
  decodeStorageScopeHeader,
  encodeStorageScopeHeader,
  storageScopeHeader,
  maxCipherBlockBytes,
} from '@sytue/config/matrix';
import { DriveClient } from '@sytue/api-clients';
import type { Bff } from './bff.js';
import { rutaBlobsMatrixPrivados } from './matrix-storage-blobs.js';
const scope = { tenant: 'acme', subject: 'person', product: 'mail' } as const,
  id = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
function fixture(
  patch: {
    path?: string;
    method?: string;
    header?: string;
    headers?: Record<string, string>;
    session?: boolean;
    query?: string;
    body?: Uint8Array;
  } = {},
) {
  const path = patch.path ?? `mail/${id}`,
    method = patch.method ?? 'GET';
  const state = { calls: [] as Request[], sessions: 0 };
  const bff = {
    getSession: async () => {
      state.sessions++;
      return patch.session === false
        ? null
        : {
            user: { id: scope.subject, tenant: scope.tenant },
            tokens: { accessToken: 'artificial-server-token' },
          };
    },
  } as unknown as Bff;
  const route = rutaBlobsMatrixPrivados({
    bff,
    env: { SYTUE_DRIVE_INTERNAL_URL: 'https://internal-drive.test' },
    product: 'mail',
    fetchImpl: (async (input, init) => {
      const request = new Request(input, init);
      state.calls.push(request);
      return new Response(method === 'DELETE' ? null : Uint8Array.of(1, 2, 3), {
        status: method === 'DELETE' ? 204 : 200,
      });
    }) as typeof fetch,
  });
  const request = new Request(
    'https://mail.test/mail/api/matrix/storage/blobs/' + path + (patch.query ?? ''),
    {
      method,
      headers: {
        [storageScopeHeader]: patch.header ?? encodeStorageScopeHeader(scope),
        Authorization: 'Bearer artificial-browser-injection',
        'X-Sytue-Tenant': 'foreign',
        ...patch.headers,
      },
      ...(patch.body ? { body: patch.body as Uint8Array<ArrayBuffer> } : {}),
    },
  );
  const ctx = {
    params: { ruta: path },
    request,
    url: new URL(request.url),
    cookies: {},
  } as unknown as APIContext;
  return { route, ctx, state };
}

test('cipher BFF streams the fixed app path with the exact session token and only its validated scope header', async () => {
  for (const operation of [
    {},
    { path: `mail/${id}/content` },
    { method: 'DELETE' },
    {
      method: 'PUT',
      body: Uint8Array.of(1, 2, 3),
      headers: { 'content-length': '3', 'content-type': 'application/octet-stream' },
    },
  ]) {
    const f = fixture(operation),
      response = await f.route(f.ctx);
    expect(response.status).toBe(operation.method === 'DELETE' ? 204 : 200);
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(response.headers.get('vary')).toBe('Cookie');
    expect(f.state.sessions).toBe(1);
    expect(f.state.calls).toHaveLength(1);
    const request = f.state.calls[0]!;
    expect(request.headers.get('authorization')).toBe('Bearer artificial-server-token');
    expect(request.headers.get('x-sytue-tenant')).toBeNull();
    expect(decodeStorageScopeHeader(request.headers.get(storageScopeHeader))).toEqual(scope);
    expect(request.url).toBe(
      'https://internal-drive.test/api/v1/cipher/blobs/' + f.ctx.params.ruta,
    );
    if (operation.method === 'PUT')
      expect(new Uint8Array(await request.arrayBuffer())).toEqual(Uint8Array.of(1, 2, 3));
    else await response.body?.cancel();
  }
});

test('wrong scope, missing consent session, extra routes, methods and invalid upload sizes never reach the gateway', async () => {
  const cases = [
    { session: false },
    { header: '' },
    { header: 'v1.invalid' },
    { header: encodeStorageScopeHeader({ ...scope, tenant: 'old' }) },
    { header: encodeStorageScopeHeader({ ...scope, subject: 'old' }) },
    { header: encodeStorageScopeHeader({ ...scope, product: 'drive' }) },
    {
      headers: {
        [storageScopeHeader]:
          encodeStorageScopeHeader(scope) + ',' + encodeStorageScopeHeader(scope),
      },
    },
    { headers: { 'sec-fetch-site': 'cross-site' } },
    { path: `drive/${id}` },
    { path: `mail/${id}/content/extra` },
    { path: `mail/${id}/../elsewhere` },
    { path: `mail/${id}%2fcontent` },
    { query: '?product=drive' },
    { method: 'POST' },
    { method: 'HEAD' },
    { method: 'DELETE', path: `mail/${id}/content` },
    { method: 'PUT', path: `mail/${id}/content` },
    { method: 'PUT' },
    {
      method: 'PUT',
      headers: {
        'content-length': String(maxCipherBlockBytes + 1),
        'content-type': 'application/octet-stream',
      },
    },
    { method: 'PUT', headers: { 'content-length': '1', 'content-type': 'application/json' } },
    {
      method: 'PUT',
      headers: {
        'content-length': '1',
        'content-type': 'application/octet-stream',
        'content-encoding': 'gzip',
      },
    },
  ];
  for (const patch of cases) {
    const f = fixture(patch),
      response = await f.route(f.ctx);
    expect(response.status).toBeGreaterThanOrEqual(400);
    expect(f.state.calls).toHaveLength(0);
    expect(response.headers.get('cache-control')).toBe('no-store');
    await response.body?.cancel();
  }
});

test('the cookie browser SDK never manufactures a bearer token for cipher downloads', async () => {
  const f = fixture({ path: `mail/${id}/content` });
  const sdk = new DriveClient({
    baseUrl: 'https://mail.test/mail',
    fetchImpl: (async (input, init) => {
      const request = new Request(input, init);
      expect(request.headers.get('authorization')).toBeNull();
      expect(request.url).toBe(f.ctx.request.url);
      return f.route({ ...f.ctx, request });
    }) as typeof fetch,
  });
  expect(await sdk.readCipherBlob(scope, id)).toEqual(Uint8Array.of(1, 2, 3));
  expect(f.state.calls).toHaveLength(1);
});

test('all fifteen product BFF routes mount only the shared private blob handler', async () => {
  const products = {
    portal: 'hub',
    mail: 'mail',
    drive: 'drive',
    docus: 'docs',
    calcus: 'sheets',
    diapos: 'slides',
    certipdf: 'pdf',
    calendar: 'calendar',
    contacts: 'contacts',
    notes: 'notes',
    photos: 'photos',
    rooms: 'rooms',
    cast: 'cast',
    erp: 'erp',
    bases: 'bases',
  };
  for (const [app, product] of Object.entries(products)) {
    const url = new URL(
      `../../../../apps/${app}/src/pages/api/matrix/storage/blobs/[...ruta].ts`,
      import.meta.url,
    );
    const source = await readFile(url, 'utf8');
    expect(source).toContain(`product: '${product}'`);
    expect(source).toContain("import { rutaBlobsMatrixPrivados } from '@sytue/auth/server'");
    expect(source).toContain('export const PUT = GET');
    expect(source).toContain('export const DELETE = GET');
    expect(source).not.toContain('accessToken');
    const bootstrapURL = new URL(
      `../../../../apps/${app}/src/pages/api/matrix/storage/bootstrap.ts`,
      import.meta.url,
    );
    const bootstrap = await readFile(bootstrapURL, 'utf8');
    const blobImport = /import \{ bff \} from '([^']+)'/.exec(source)?.[1];
    const bootImport = /import \{ bff \} from '([^']+)'/.exec(bootstrap)?.[1];
    expect(blobImport).toBeDefined();
    expect(bootImport).toBeDefined();
    const resolved = (specifier: string, base: URL) =>
      specifier.startsWith('.') ? new URL(specifier, base).href : specifier;
    expect(resolved(blobImport!, url)).toBe(resolved(bootImport!, bootstrapURL));
  }
});
