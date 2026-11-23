/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import type { Bff } from './bff.js';
import { rutaBovedaPq } from './pq-vault.js';

function fakeBff(session: {
  tokens: { accessToken: string };
  user: { id: string; tenant: string };
}): Bff {
  return {
    getSession: async () => session as never,
  } as unknown as Bff;
}

describe('rutaBovedaPq', () => {
  const session = {
    tokens: { accessToken: 'tok-1' },
    user: { id: 'sub-alice', tenant: 'acme' },
  };
  const env = {
    SYTUE_DRIVE_INTERNAL_URL: 'http://drive.internal',
    SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
  };

  test('bootstrap devuelve metadatos de sesión sin llamar al backend', async () => {
    let called = 0;
    const handler = rutaBovedaPq({
      bff: fakeBff(session),
      env,
      product: 'drive',
      fetchImpl: (async () => {
        called++;
        return new Response('unexpected', { status: 500 });
      }) as unknown as typeof fetch,
    });
    const req = new Request('https://drive.sytue.eu/api/pq-vault/bootstrap', { method: 'GET' });
    const res = await handler({
      request: req,
      url: new URL(req.url),
      params: { ruta: 'bootstrap' },
      cookies: {} as never,
    } as never);
    expect(called).toBe(0);
    expect(res.status).toBe(200);
    expect(res.headers.get('cache-control')).toBe('no-store');
    expect(await res.json()).toEqual({
      actor: 'sub-alice',
      tenant: 'acme',
      product: 'drive',
      id_origin: 'https://id.sytue.eu',
      encryption: 'age-MLKEM768-X25519-v1',
    });
  });

  test('bootstrap respeta SYTUE_CIFRADO_E2E=desactivado', async () => {
    const handler = rutaBovedaPq({
      bff: fakeBff(session),
      env: { ...env, SYTUE_CIFRADO_E2E: 'desactivado' },
      product: 'notes',
    });
    const req = new Request('https://notes.sytue.eu/api/pq-vault/bootstrap', { method: 'GET' });
    const res = await handler({
      request: req,
      url: new URL(req.url),
      params: { ruta: 'bootstrap' },
      cookies: {} as never,
    } as never);
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ cifrado: 'desactivado' });
  });

  test('proxya keys y records a services/drive verificando el scope header', async () => {
    const seen: { url: string; method: string; scope: string | null }[] = [];
    const handler = rutaBovedaPq({
      bff: fakeBff(session),
      env,
      product: 'drive',
      fetchImpl: (async (input: RequestInfo | URL, init?: RequestInit) => {
        seen.push({
          url: String(input),
          method: init?.method ?? 'GET',
          scope: new Headers(init?.headers).get(storageScopeHeader),
        });
        return Response.json({ vault_id: '!pqv_AAAAAAAAAAAAAAAAAAAAAAAAAA', keys: [] });
      }) as unknown as typeof fetch,
    });
    const scopeHeader = encodeStorageScopeHeader({
      tenant: 'acme',
      subject: 'sub-alice',
      product: 'drive',
    });
    const req = new Request('https://drive.sytue.eu/api/pq-vault/keys', {
      method: 'GET',
      headers: { [storageScopeHeader]: scopeHeader },
    });
    const res = await handler({
      request: req,
      url: new URL(req.url),
      params: { ruta: 'keys' },
      cookies: {} as never,
    } as never);
    expect(res.status).toBe(200);
    expect(seen).toEqual([
      {
        url: 'http://drive.internal/api/v1/pq-vault/drive/keys',
        method: 'GET',
        scope: scopeHeader,
      },
    ]);
  });

  test('signing-key va a Sytue ID con el token de la persona, solo GET y PUT', async () => {
    const seen: { url: string; method: string; auth: string | null }[] = [];
    const handler = rutaBovedaPq({
      bff: fakeBff(session),
      env: { ...env, SYTUE_OIDC_INTERNAL_BASE_URL: 'http://sytue-one-id' },
      product: 'mail',
      fetchImpl: (async (input: RequestInfo | URL, init?: RequestInit) => {
        seen.push({
          url: String(input),
          method: init?.method ?? 'GET',
          auth: new Headers(init?.headers).get('authorization'),
        });
        return Response.json({ keys: [] });
      }) as unknown as typeof fetch,
    });
    const call = (method: string, init: RequestInit = {}) => {
      const req = new Request('https://mail.sytue.eu/api/pq-vault/signing-key', {
        method,
        ...init,
      });
      return handler({
        request: req,
        url: new URL(req.url),
        params: { ruta: 'signing-key' },
        cookies: {} as never,
      } as never) as Promise<Response>;
    };
    expect((await call('GET')).status).toBe(200);
    expect(seen).toEqual([
      { url: 'http://sytue-one-id/api/v1/me/vault-keys/mail', method: 'GET', auth: 'Bearer tok-1' },
    ]);
    expect((await call('DELETE')).status).toBe(405);
    // Desde otro sitio, nada.
    const ajena = await call('GET', { headers: { origin: 'https://evil.example' } });
    expect(ajena.status).toBe(403);
    expect(seen.length).toBe(1);
  });

  test('copy: solo DELETE con ?id= de la marca, hacia services/drive', async () => {
    const seen: { url: string; method: string }[] = [];
    const handler = rutaBovedaPq({
      bff: fakeBff(session),
      env,
      product: 'drive',
      fetchImpl: (async (input: RequestInfo | URL, init?: RequestInit) => {
        seen.push({ url: String(input), method: init?.method ?? 'GET' });
        return Response.json({ vault_id: '!pqv_AAAAAAAAAAAAAAAAAAAAAAAAAA', keys: [] });
      }) as unknown as typeof fetch,
    });
    const scopeHeader = encodeStorageScopeHeader({
      tenant: 'acme',
      subject: 'sub-alice',
      product: 'drive',
    });
    const call = (method: string, search: string) => {
      const req = new Request(`https://drive.sytue.eu/api/pq-vault/copy${search}`, {
        method,
        headers: { [storageScopeHeader]: scopeHeader },
      });
      return handler({
        request: req,
        url: new URL(req.url),
        params: { ruta: 'copy' },
        cookies: {} as never,
      } as never);
    };
    const id = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
    expect((await call('DELETE', `?id=${id}`)).status).toBe(200);
    expect(seen).toEqual([
      { url: `http://drive.internal/api/v1/pq-vault/drive/copy?id=${id}`, method: 'DELETE' },
    ]);
    expect((await call('PUT', `?id=${id}`)).status).toBe(405);
    expect((await call('DELETE', '')).status).toBe(400);
    expect((await call('DELETE', `?id=${id}&otra=1`)).status).toBe(400);
    expect((await call('DELETE', '?id=corto')).status).toBe(400);
    expect(seen).toHaveLength(1);
  });

  test('rechaza scope de otro producto con 409', async () => {
    const handler = rutaBovedaPq({
      bff: fakeBff(session),
      env,
      product: 'drive',
    });
    const wrongScope = encodeStorageScopeHeader({
      tenant: 'acme',
      subject: 'sub-alice',
      product: 'notes',
    });
    const req = new Request('https://drive.sytue.eu/api/pq-vault/keys', {
      method: 'GET',
      headers: { [storageScopeHeader]: wrongScope },
    });
    const res = await handler({
      request: req,
      url: new URL(req.url),
      params: { ruta: 'keys' },
      cookies: {} as never,
    } as never);
    expect(res.status).toBe(409);
  });
});
