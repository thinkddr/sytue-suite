import { afterAll, beforeAll, describe, expect, test } from 'bun:test';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { createStaticHandler } from '../../static-serve.mjs';

let root = '';

beforeAll(async () => {
  root = await mkdtemp(join(tmpdir(), 'sytue-www-'));
  await mkdir(join(root, '_astro'));
  await mkdir(join(root, 'de'));
  await writeFile(join(root, 'de', 'index.html'), '<!doctype html><title>Sytue DE</title>');
  await writeFile(join(root, 'index.html'), '<!doctype html><title>Sytue</title>');
  await writeFile(join(root, '404.html'), '<!doctype html><title>No encontrado</title>');
  await writeFile(join(root, '_astro', 'app.js'), 'console.log("Sytue")');
});

afterAll(async () => {
  await rm(root, { recursive: true, force: true });
});

describe('servidor estático de producción', () => {
  test('sirve HTML con la política de seguridad completa', async () => {
    const response = await createStaticHandler(root)(new Request('https://www.sytue.eu/'));

    expect(response.status).toBe(200);
    expect(response.headers.get('content-security-policy')).toContain("default-src 'self'");
    expect(response.headers.get('content-security-policy')).toContain("object-src 'none'");
    expect(response.headers.get('strict-transport-security')).toContain('includeSubDomains');
    expect(response.headers.get('x-content-type-options')).toBe('nosniff');
    expect(response.headers.get('x-frame-options')).toBe('DENY');
    expect(response.headers.get('referrer-policy')).toBe('strict-origin-when-cross-origin');
    expect(response.headers.get('permissions-policy')).toContain('camera=()');
    expect(await response.text()).toContain('<title>Sytue</title>');
  });

  test('rechaza escritura y traversal conservando las cabeceras', async () => {
    const handler = createStaticHandler(root);
    const post = await handler(new Request('https://www.sytue.eu/', { method: 'POST' }));
    const traversal = await handler(new Request('https://www.sytue.eu/%2e%2e%2fsecret'));

    expect(post.status).toBe(405);
    expect(post.headers.get('allow')).toBe('GET, HEAD');
    expect(post.headers.get('content-security-policy')).not.toBeNull();
    expect(traversal.status).toBe(400);
    expect(traversal.headers.get('x-frame-options')).toBe('DENY');
  });

  test('cachea solo los assets compilados como inmutables', async () => {
    const response = await createStaticHandler(root)(
      new Request('https://www.sytue.eu/_astro/app.js'),
    );

    expect(response.status).toBe(200);
    expect(response.headers.get('cache-control')).toBe('public, max-age=31536000, immutable');
  });
});

describe('idioma de la raíz', () => {
  const get = (path: string, headers: Record<string, string> = {}) =>
    createStaticHandler(root)(new Request(`https://sytue.eu${path}`, { headers }));

  test.each([
    ['de-DE,de;q=0.9', '/de'],
    ['de-AT', '/de'],
    ['pt-BR,en;q=0.5', '/pt'],
    ['es-MX', '/es'],
    ['ja,fr;q=0.3', '/fr'],
  ])('Accept-Language %s → 302 a %s', async (al, destino) => {
    const res = await get('/?utm=x', { 'accept-language': al });
    expect(res.status).toBe(302);
    expect(res.headers.get('location')).toBe(`${destino}?utm=x`);
    expect(res.headers.get('vary')).toContain('Accept-Language');
    expect(res.headers.get('cache-control')).toBe('private, no-cache');
  });

  test('sin preferencia, o solo inglés, sirve la raíz en inglés con su Vary', async () => {
    for (const headers of [{}, { 'accept-language': 'en-US' }, { 'accept-language': 'ja' }]) {
      const res = await get('/', headers);
      expect(res.status).toBe(200);
      expect(res.headers.get('vary')).toContain('Cookie');
      expect(res.headers.get('cache-control')).toBe('private, no-cache');
    }
  });

  test('la cookie del selector manda sobre la cabecera, también para volver al inglés', async () => {
    const en = await get('/', { 'accept-language': 'de', cookie: 'sytue_locale=en' });
    const fr = await get('/', { 'accept-language': 'de', cookie: 'a=1; sytue_locale=fr' });
    expect(en.status).toBe(200);
    expect(fr.headers.get('location')).toBe('/fr');
  });

  test('a los rastreadores no se les redirige por cabecera', async () => {
    const res = await get('/', {
      'accept-language': 'de',
      'user-agent': 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
    });
    expect(res.status).toBe(200);
  });

  test('solo negocia la raíz: una URL con idioma se sirve tal cual', async () => {
    const res = await get('/de', { 'accept-language': 'fr', cookie: 'sytue_locale=it' });
    expect(res.status).toBe(200);
    expect(res.headers.get('vary')).toBeNull();
    expect(res.headers.get('cache-control')).toBe('public, max-age=300');
  });
});
