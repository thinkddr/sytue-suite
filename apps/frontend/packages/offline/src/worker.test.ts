import { describe, expect, test } from 'bun:test';
import { nativeWorkerResponse, RE_BASE_DE_APP } from './worker.js';

describe('nativeWorkerResponse', () => {
  test('se sirve como worker no cacheable y con alcance de raíz', async () => {
    const response = nativeWorkerResponse();
    expect(response.headers.get('content-type')).toContain('text/javascript');
    expect(response.headers.get('cache-control')).toContain('no-store');
    expect(response.headers.get('service-worker-allowed')).toBe('/');
    const source = await nativeWorkerResponse().text();
    expect(() => new Function(source)).not.toThrow();
  });

  test('aísla por cuenta y purga datos privados al cerrar sesión', async () => {
    const source = await nativeWorkerResponse().text();
    expect(source).toContain("ownerCache('pages', context)");
    expect(source).toContain("ownerCache('api', context)");
    expect(source).toContain("data.type !== 'INITIALISE'");
    expect(source).toContain("request.destination === 'script'");
    expect(source).toContain("indexedDB.deleteDatabase('sytue-offline')");
    expect(source).toContain('SYTUE_CLEAR_OFFLINE');
    expect(source).toContain('includeUncontrolled: true');
    expect(source).toContain("sinBase(url.pathname).startsWith('/auth/logout')");
    expect(source).toContain('SYTUE_NETWORK_STATE');
  });

  test('no intercepta secretos de sesión y limita archivos cacheados', async () => {
    const source = await nativeWorkerResponse().text();
    expect(source).toContain("'/api/token'");
    expect(source).toContain("'/api/session'");
    expect(source).toContain("'/api/ws-ticket'");
    expect(source).toContain('50 * 1024 * 1024');
    expect(source).toContain('explicitlyOffline && (!Number.isFinite(length) || length <= 0)');
    expect(source).toContain('allowPrivateNoStore');
    expect(source).toContain('precachePrivate');
    expect(source).toContain("request.headers.has('range')");
  });
});

/*
 * El PREFIJO DE BASE, comprobado por comportamiento y no por la letra.
 *
 * El test de arriba afirmaba que el código fuente CONTENÍA
 * `url.pathname.startsWith('/auth/logout')`. Eso estuvo verde todo el tiempo que
 * esa rama no llegó a ejecutarse nunca en producción, porque la ruta real dentro
 * de la ventana nativa es `/mail/auth/logout`. Una aserción sobre el texto del
 * programa no dice nada sobre lo que el programa hace.
 */
describe('RE_BASE_DE_APP', () => {
  const sinBase = (p: string): string => p.replace(RE_BASE_DE_APP, '');

  test('recorta el prefijo de la app en las rutas internas', () => {
    expect(sinBase('/mail/auth/logout')).toBe('/auth/logout');
    expect(sinBase('/one/api/token')).toBe('/api/token');
    expect(sinBase('/cloud-dashboard/_astro/x.js')).toBe('/_astro/x.js');
  });

  test('sin prefijo, no toca nada (la web y el shell antiguo siguen igual)', () => {
    expect(sinBase('/auth/logout')).toBe('/auth/logout');
    expect(sinBase('/api/token')).toBe('/api/token');
  });

  // LO QUE NO PUEDE RECORTAR. Comerse un segmento de una ruta de contenido
  // mandaría a cachear con la clave equivocada, que es peor que el fallo original.
  test('no toca las rutas de contenido', () => {
    expect(sinBase('/docs/edit')).toBe('/docs/edit');
    expect(sinBase('/drive/files/abc')).toBe('/drive/files/abc');
    expect(sinBase('/one/auth')).toBe('/one/auth');
    expect(sinBase('/api/tokenizer')).toBe('/api/tokenizer');
  });
});
