/**
 * Tests de HttpError: leer el `code` estable del error de API en vez del texto.
 *
 * Lo que se guarda aquí es que un 403 por FALTA DE SUSCRIPCIÓN se pueda
 * distinguir de un 403 de permisos. No son lo mismo para quien lo recibe: al
 * primero se responde completando la suscripción, al segundo pidiendo acceso.
 * Mientras los dos llegaban como «forbidden», las diez apps mostraban el mismo
 * «no se pudieron cargar los datos» y el usuario no tenía ninguna pista.
 *
 * Ejecutar: `bun test`.
 */
import { describe, expect, test } from 'bun:test';
import { HttpClient, HttpError, cuerpoDeError, sameOriginUrl, upstreamStatus } from './http.js';

function error(status: number, body?: string): HttpError {
  return new HttpError(status, 'Forbidden', 'https://mail.sytue.eu/api/v1/labels', body);
}

test('lee el code del cuerpo JSON', () => {
  const e = error(403, JSON.stringify({ code: 'no_active_organization', message: 'x' }));
  expect(e.code).toBe('no_active_organization');
  expect(e.noActiveOrg).toBe(true);
});

test('un 403 de permisos NO se confunde con la falta de organización', () => {
  const e = error(403, JSON.stringify({ code: 'forbidden', message: 'x' }));
  expect(e.code).toBe('forbidden');
  expect(e.noActiveOrg).toBe(false);
});

test('el estado tiene que acompañar al código', () => {
  // Un cuerpo con ese código pero otro estado no es el caso que buscamos: se
  // exige la pareja para que un eco o un proxy no lo activen por su cuenta.
  const e = error(500, JSON.stringify({ code: 'no_active_organization' }));
  expect(e.noActiveOrg).toBe(false);
});

test('un cuerpo que no es JSON no rompe nada', () => {
  // Traefik y los ingress devuelven HTML o texto plano en sus propios errores.
  // Si esto lanzara, el fallo de red se convertiría en una excepción distinta y
  // el mensaje que ve el usuario sería aún menos parecido a la causa.
  expect(error(403, '<html>504 Gateway Timeout</html>').code).toBeUndefined();
  expect(error(403, '').code).toBeUndefined();
  expect(error(403).code).toBeUndefined();
  expect(error(403, 'null').code).toBeUndefined();
  expect(error(403, '{"code":123}').code).toBeUndefined();
});

test('lee el code envuelto que emite httpx.Fail', () => {
  // Es lo que responde de verdad un servicio, y el proxy del BFF no lo toca.
  const cuerpo = { error: { status: 409, code: 'rev_conflict', message: 'x' }, request_id: 'r' };
  expect(error(409, JSON.stringify(cuerpo)).code).toBe('rev_conflict');
  expect(
    error(403, JSON.stringify({ error: { code: 'no_active_organization' } })).noActiveOrg,
  ).toBe(true);
  // Un `error` que es texto no es el sobre: se sigue leyendo la raíz.
  expect(error(400, '{"error":"mal","code":"bad_request"}').code).toBe('bad_request');
  expect(error(400, '{"error":{"code":7}}').code).toBeUndefined();
});

test('cuerpoDeError lee código y mensaje del sobre y de la raíz', () => {
  // El sobre es lo que contesta el motor de PDF a un fichero con contraseña: el BFF de
  // CertiPDF leía solo la raíz y la web enseñaba «pdf_failed» en vez de pedirla.
  const sobre = { error: { code: 'pdf_password_required', message: 'needs a password' } };
  expect(cuerpoDeError(JSON.stringify(sobre))).toEqual({
    code: 'pdf_password_required',
    message: 'needs a password',
  });
  expect(cuerpoDeError('{"code":"too_large","message":"x"}')).toEqual({
    code: 'too_large',
    message: 'x',
  });
  expect(cuerpoDeError('{"error":"mal","code":"bad_request"}').code).toBe('bad_request');
  for (const roto of [undefined, '', 'null', '<html>502</html>', '{"error":{"code":7}}']) {
    expect(cuerpoDeError(roto).code).toBeUndefined();
  }
});

test('upstreamStatus: el 4xx del servicio pasa tal cual y lo demás es 502', () => {
  // Visto en producción: retirada la compartición, el servicio da 403 y el BFF lo
  // convertía en 502, que la web toma por una caída.
  for (const s of [401, 403, 404, 409, 412]) expect(upstreamStatus(error(s))).toBe(s);
  expect(upstreamStatus(error(500))).toBe(502);
  expect(upstreamStatus(error(503))).toBe(502);
  expect(upstreamStatus(new TypeError('fetch failed'))).toBe(502);
  expect(upstreamStatus(undefined)).toBe(502);
});

/*
 * QUERY preferente con GET de respaldo.
 *
 * Lo que se sujeta aquí es el comportamiento que cuesta ver en producción: que
 * un servidor viejo no pague un viaje de más en CADA consulta, y que un error
 * que no habla de QUERY no acabe convertido en dos peticiones.
 */
describe('consulta: QUERY preferente', () => {
  const recogerLlamadas = (respuestas: Array<{ status: number; body?: string }>) => {
    const llamadas: Array<{ method: string; url: string; body: string | null }> = [];
    let i = 0;
    const fetchImpl = (async (url: string | URL, init?: RequestInit) => {
      const r = respuestas[Math.min(i, respuestas.length - 1)]!;
      i++;
      llamadas.push({
        method: init?.method ?? 'GET',
        url: String(url),
        body: (init?.body as string | null) ?? null,
      });
      return new Response(r.body ?? '{"ok":true}', {
        status: r.status,
        headers: { 'content-type': 'application/json' },
      });
    }) as unknown as typeof fetch;
    return { llamadas, fetchImpl };
  };

  test('usa QUERY y manda los parámetros en el cuerpo, no en la URL', async () => {
    const { llamadas, fetchImpl } = recogerLlamadas([{ status: 200 }]);
    const c = new HttpClient({ baseUrl: 'https://uno.example', fetchImpl });
    await c.consulta({ path: '/search', query: { q: 'playa', limit: 20 } });

    expect(llamadas).toHaveLength(1);
    expect(llamadas[0]!.method).toBe('QUERY');
    expect(llamadas[0]!.body).toBe('q=playa&limit=20');
    // Y la URL queda limpia: es medio motivo de haber hecho esto.
    expect(llamadas[0]!.url).not.toContain('playa');
  });

  test('si el servidor no lo entiende, cae a GET y NO lo reintenta nunca más', async () => {
    const { llamadas, fetchImpl } = recogerLlamadas([
      { status: 405 },
      { status: 200 },
      { status: 200 },
    ]);
    const c = new HttpClient({ baseUrl: 'https://viejo.example', fetchImpl });

    await c.consulta({ path: '/search', query: { q: 'playa' } });
    expect(llamadas.map((l) => l.method)).toEqual(['QUERY', 'GET']);

    // La segunda consulta ya va directa: probar cada vez costaría un viaje por
    // consulta contra el servidor que menos conviene cargar.
    await c.consulta({ path: '/search', query: { q: 'mar' } });
    expect(llamadas.map((l) => l.method)).toEqual(['QUERY', 'GET', 'GET']);
    expect(llamadas[2]!.url).toContain('q=mar');
  });

  test('el 404 de un BFF de Astro sin handler QUERY también cae a GET', async () => {
    // Astro contesta 404 —y no 405— a un método sin handler: `mod[method] ??
    // mod.ALL` en dist/runtime/server/endpoint.js. Sin esto, buscar contra un
    // BFF que aún no hable QUERY fallaba en la cara de quien buscaba.
    const { llamadas, fetchImpl } = recogerLlamadas([{ status: 404 }, { status: 200 }]);
    const c = new HttpClient({ baseUrl: 'https://astro.example', fetchImpl });

    await c.consulta({ path: '/api/search', query: { q: 'playa' } });
    expect(llamadas.map((l) => l.method)).toEqual(['QUERY', 'GET']);
  });

  test('si el respaldo también da 404, el error es suyo y no se apunta nada', async () => {
    // Un 404 es igual de compatible con «esa ruta no existe», y apuntarlo
    // dejaría a ese servidor sin QUERY para siempre por una ruta mal escrita.
    const { llamadas, fetchImpl } = recogerLlamadas([
      { status: 404 },
      { status: 404 },
      { status: 200 },
    ]);
    const c = new HttpClient({ baseUrl: 'https://ruta-mala.example', fetchImpl });

    await expect(c.consulta({ path: '/no-existe', query: { q: 'playa' } })).rejects.toThrow();
    // La siguiente vuelve a intentar QUERY: no se aprendió nada falso.
    await c.consulta({ path: '/search', query: { q: 'mar' } });
    expect(llamadas.map((l) => l.method)).toEqual(['QUERY', 'GET', 'QUERY']);
  });

  test('un error que no habla de QUERY se propaga, no se reintenta', async () => {
    const { llamadas, fetchImpl } = recogerLlamadas([{ status: 500 }]);
    const c = new HttpClient({ baseUrl: 'https://roto.example', fetchImpl });
    await expect(c.consulta({ path: '/search', query: { q: 'playa' } })).rejects.toThrow();
    expect(llamadas).toHaveLength(1);
  });

  test('sin parámetros no hay nada que mover al cuerpo: va por GET', async () => {
    const { llamadas, fetchImpl } = recogerLlamadas([{ status: 200 }]);
    const c = new HttpClient({ baseUrl: 'https://otro.example', fetchImpl });
    await c.consulta({ path: '/quota' });
    expect(llamadas[0]!.method).toBe('GET');
  });
});

describe('sameOriginUrl: el recurso no puede salir del servicio configurado', () => {
  const base = 'https://calendar.sytue.eu';

  test('una ruta del servicio pasa, resuelta a absoluta', () => {
    expect(sameOriginUrl(base, '/dav/calendars/u/trabajo/')).toBe(
      'https://calendar.sytue.eu/dav/calendars/u/trabajo/',
    );
  });

  test('un absoluto del propio servicio pasa', () => {
    expect(sameOriginUrl(base, 'https://calendar.sytue.eu/dav/x')).toBe(
      'https://calendar.sytue.eu/dav/x',
    );
  });

  test('un absoluto a otro host se rechaza (robo de token / SSRF)', () => {
    // Antes del arreglo esto se devolvía tal cual y el Bearer viajaba al atacante.
    expect(() => sameOriginUrl(base, 'http://127.0.0.1:18091/robo')).toThrow();
    expect(() => sameOriginUrl(base, 'https://evil.example/dav')).toThrow();
  });

  test('el protocolo-relativo //host también se rechaza', () => {
    expect(() => sameOriginUrl(base, '//evil.example/dav')).toThrow();
  });
});

// External API throttling must not cause a busy loop or an automatic duplicate POST.
test('provider cooldown reads wrapped params and rejects malformed delays', () => {
  expect(error(429, '{"error":{"params":{"retry_after_seconds":120}}}').retryAfterMs).toBe(120000);
  for (const delay of [-1, 0, '120', null])
    expect(
      error(429, JSON.stringify({ params: { retry_after_seconds: delay } })).retryAfterMs,
    ).toBe(0);
  expect(error(429, '{"params":{"retry_after_seconds":999999}}').retryAfterMs).toBe(86400000);
});
