// Lo que el SDK de Docs entiende de la lista de documentos, y qué pide al listar:
// «Míos / Compartidos conmigo» y el orden por título los resuelve el servicio
// (`?scope=`, `?sort=title`), nunca el cliente sobre la página traída. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { DocsClient } from './index.js';

function clienteCon(cuerpo: unknown): DocsClient {
  const fetchImpl = (async () =>
    new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as unknown as typeof fetch;
  return new DocsClient({
    baseUrl: 'https://docs.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl,
  });
}

function clienteQueApunta(): { cliente: DocsClient; consultas: URLSearchParams[] } {
  const consultas: URLSearchParams[] = [];
  const fetchImpl = (async (url: string | URL) => {
    consultas.push(new URL(String(url)).searchParams);
    return new Response(JSON.stringify({ documents: [], next_cursor: 't~QQ~x' }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  return {
    cliente: new DocsClient({
      baseUrl: 'https://docs.test',
      auth: { getAccessToken: () => 'tok' },
      fetchImpl,
    }),
    consultas,
  };
}

const BASE = {
  title: 'Acta',
  kind: 'document',
  owner_sub: 'ada',
  seq: 9,
  created_at: '2026-11-01T10:00:00Z',
  updated_at: '2026-11-02T10:00:00Z',
};

// La secuencia de la miniatura es lo que la portada pone en la URL de la imagen; sin
// ella (0 o ausente) no hay miniatura y se pinta el glifo del tipo.
test('la lista trae la secuencia de la miniatura de la portada', async () => {
  const { documents } = await clienteCon({
    documents: [
      { ...BASE, id: 'a', thumbnail_seq: 7 },
      { ...BASE, id: 'b', thumbnail_seq: 0 },
      { ...BASE, id: 'c' },
    ],
  }).list();
  expect(documents.map((d) => d.thumbnailSeq)).toEqual([7, undefined, undefined]);
});

test('list pasa el alcance y el orden por título al servicio', async () => {
  const { cliente, consultas } = clienteQueApunta();
  const pagina = await cliente.list({ scope: 'shared', sort: 'title' });
  expect(consultas[0]?.get('scope')).toBe('shared');
  expect(consultas[0]?.get('sort')).toBe('title');
  expect(pagina.nextCursor).toBe('t~QQ~x');
});

test('sin alcance ni orden, la consulta es la de siempre', async () => {
  const { cliente, consultas } = clienteQueApunta();
  await cliente.list({ sort: 'updated' });
  expect(consultas[0]?.has('scope')).toBe(false);
  expect(consultas[0]?.has('sort')).toBe(false);
});
