// Checks de lo que el SDK deja pasar de la búsqueda federada. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { HubClient } from './index.js';

/** Un Hub de mentira que devuelve exactamente lo que se le diga. */
// Se captura LA PETICIÓN ENTERA —URL y cuerpo— y no solo la URL: desde que la
// búsqueda va por QUERY (RFC 10008), los parámetros viajan en el cuerpo. Lo que
// estos tests comprueban es que el dato SE MANDA, no por dónde; mirar solo la
// URL los ataba a un detalle de transporte y los rompió al cambiarlo.
function hubQueResponde(cuerpo: unknown, capturarUrl?: (u: string) => void): HubClient {
  return new HubClient({
    baseUrl: 'https://hub.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl: ((url: string, init?: RequestInit) => {
      capturarUrl?.(`${String(url)} ${String(init?.body ?? '')}`);
      return Promise.resolve(
        new Response(JSON.stringify(cuerpo), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        }),
      );
    }) as typeof fetch,
  });
}

test('un origen caído se cuenta, en vez de convertirse en «no hay resultados»', async () => {
  // ES EL BUG: la búsqueda federada degrada con elegancia a propósito —un origen
  // caído no rompe el resto—, pero el SDK descartaba `sources[].error`. La
  // degradación se volvía una MENTIRA: quien buscaba daba por perdido un correo
  // que sí existe.
  const hub = hubQueResponde({
    query: 'factura',
    hits: [{ source: 'drive', id: '1', title: 'Factura.pdf', url: '/f/1' }],
    sources: [
      { source: 'drive', took_ms: 12 },
      { source: 'mail', took_ms: 3000, error: 'timeout' },
    ],
    total: 1,
  });
  const r = await hub.search('factura');
  expect(r.results).toHaveLength(1);
  expect(r.fallidos).toEqual([{ source: 'mail', error: 'timeout' }]);
});

test('todo en orden deja la lista de fallidos vacía', async () => {
  const hub = hubQueResponde({
    query: 'x',
    hits: [],
    sources: [{ source: 'drive', took_ms: 5 }],
    total: 0,
  });
  expect((await hub.search('x')).fallidos).toEqual([]);
});

test('el total y el «hay más» llegan a quien pinta la lista', async () => {
  // Sin ellos la interfaz enseña la primera tanda y calla, sin manera de saber
  // que hay otras: ni «23 resultados» ni «ver más».
  const hub = hubQueResponde({
    query: 'x',
    hits: [{ source: 'mail', id: 'a', title: 'A', url: '/a' }],
    total: 23,
    has_more: true,
  });
  const r = await hub.search('x');
  expect(r.total).toBe(23);
  expect(r.hasMore).toBe(true);
});

test('un Hub que no manda total no inventa uno: cuenta lo que ha traído', async () => {
  const hub = hubQueResponde({
    query: 'x',
    hits: [
      { source: 'mail', id: 'a', title: 'A', url: '/a' },
      { source: 'mail', id: 'b', title: 'B', url: '/b' },
    ],
  });
  const r = await hub.search('x');
  expect(r.total).toBe(2);
  expect(r.hasMore).toBe(false);
});

test('la paginación viaja en la petición', async () => {
  let vista = '';
  const hub = hubQueResponde({ query: 'x', hits: [] }, (u) => (vista = u));
  await hub.search('x', undefined, { limit: 20, offset: 40 });
  expect(vista).toContain('limit=20');
  expect(vista).toContain('offset=40');
});

// `ia=1` solo se manda cuando se pide: la búsqueda de siempre no puede cambiar
// de coste porque exista la opción.
test('la interpretación se pide a propósito y se aplana con las fechas en ms', async () => {
  let visto = '';
  const hub = hubQueResponde(
    {
      query: 'factura Acme',
      hits: [],
      interpreted: {
        text: 'factura Acme',
        sources: ['drive'],
        kinds: ['file'],
        from: '2026-08-01T00:00:00Z',
      },
    },
    (u) => {
      visto = u;
    },
  );

  const r = await hub.search('facturas de Acme del mes pasado', undefined, { ia: true });
  expect(visto).toContain('ia=1');
  expect(r.interpretado?.sources).toEqual(['drive']);
  expect(r.interpretado?.from).toBe(Date.parse('2026-08-01T00:00:00Z'));
});

// Sin pedirla, no llega interpretación y la URL no la menciona.
test('la búsqueda de siempre no manda ia', async () => {
  let visto = '';
  const hub = hubQueResponde({ query: 'x', hits: [] }, (u) => {
    visto = u;
  });
  const r = await hub.search('x');
  expect(visto).not.toContain('ia=1');
  expect(r.interpretado).toBeUndefined();
});

test('el tamaño de un fichero llega como número, para decirlo en el idioma de quien busca', async () => {
  // El Hub escribe el tamaño en el snippet siempre igual («11.7 KiB»): salía en inglés
  // y con otras unidades que el resto de la suite. Visto en una ventana real.
  const hub = hubQueResponde({
    query: 'acta',
    hits: [
      {
        source: 'drive',
        id: '1',
        title: 'acta.pdf',
        snippet: '11.7 KiB',
        size_bytes: 11980,
        url: '/f/1',
      },
    ],
    sources: [{ source: 'drive', took_ms: 4 }],
    total: 1,
  });
  const r = await hub.search('acta');
  expect(r.results[0]?.sizeBytes).toBe(11980);
});
