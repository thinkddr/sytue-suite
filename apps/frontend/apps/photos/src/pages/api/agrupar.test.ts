/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/agrupar: UNA llamada de texto a Sytue AI con lo ya visto de cada foto,
 * y de vuelta la propuesta con los ids DE VERDAD.
 *
 * Aquí sí se importa la ruta, con la sesión de mentira y `fetch` sustituido, y no
 * se lee su fuente como en `describir.test.ts`: lo que importa es lo que sale por
 * la red —sin imagen, sin razonar, en el idioma de la sesión— y cómo se traducen
 * los números que lee el modelo a los ids de las fotos.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';
import { contextoAgrupar, instruccionAgrupar, TOPE_ORGANIZAR } from '../../lib/agruparIA.js';

let conSesion = true;
mock.module('../../lib/bff.js', () => ({
  bff: {
    getSession: () =>
      Promise.resolve(conSesion ? { tokens: { accessToken: 'token-de-ana' } } : null),
  },
  toBffContext: (c: unknown) => c,
}));
// La base de Sytue AI se lee al cargar `visionIA.ts`: va antes de importar la ruta.
process.env['SYTUE_AI_URL'] = 'http://ia.test';
const { POST } = await import('./agrupar.js');

const fetchReal = globalThis.fetch;
let pedidas: { url: string; init: RequestInit }[] = [];
let contesta = '';
afterEach(() => {
  globalThis.fetch = fetchReal;
  pedidas = [];
  conSesion = true;
});
function iaQueContesta(texto: string): void {
  contesta = texto;
  globalThis.fetch = ((url: string, init: RequestInit) => {
    pedidas.push({ url, init });
    return Promise.resolve(Response.json({ text: contesta }));
  }) as unknown as typeof fetch;
}

function agrupar(cuerpo: unknown): Promise<Response> {
  const request = new Request('https://photos.sytue.eu/api/agrupar', {
    method: 'POST',
    body: JSON.stringify(cuerpo),
  });
  return POST({ request, locals: { locale: 'es' } } as unknown as APIContext) as Promise<Response>;
}

const fotos = [
  {
    id: 'foto-a',
    description: 'Una cala al atardecer',
    tags: ['playa'],
    takenAtFromExif: true,
    takenAt: '2024-07-14T10:00:00Z',
  },
  { id: 'foto-sin-nada' },
  { id: 'foto-b', tags: ['playa', 'barca'], location: { lat: 36.7, lon: -4.4 } },
  { id: 'foto-c', description: 'Nieve en la sierra' },
];

test('manda una llamada de texto y devuelve los ids de las fotos, no los números', async () => {
  iaQueContesta(
    '```json\n{"albumes":[{"titulo":"Playa","ids":["2","1"]},{"titulo":"Sola","ids":["3"]},{"titulo":"Inventado","ids":["7","8"]}]}\n```',
  );
  const res = await agrupar({ fotos });
  expect(res.status).toBe(200);
  expect(await res.json()).toEqual({ albumes: [{ titulo: 'Playa', ids: ['foto-b', 'foto-a'] }] });

  expect(pedidas.length).toBe(1);
  expect(pedidas[0]!.url).toBe('http://ia.test/api/v1/assist');
  const cabeceras = new Headers(pedidas[0]!.init.headers);
  expect(cabeceras.get('authorization')).toBe('Bearer token-de-ana');
  expect(cabeceras.get('accept-language')).toBe('es');
  const cuerpo = JSON.parse(pedidas[0]!.init.body as string) as Record<string, unknown>;
  expect(cuerpo).toEqual({
    action: 'agrupar-fotos',
    kind: 'photos',
    instruction: instruccionAgrupar('es'),
    context: contextoAgrupar(fotos).contexto,
    esfuerzo: 'none',
  });
  // Lo que viaja es lo que se vio, con números cortos: ni la foto ni su id.
  expect(cuerpo['context']).not.toContain('foto-');
});

test('sin sesión, o con un cuerpo que no es una lista de fotos, no se gasta nada', async () => {
  iaQueContesta('{"albumes":[]}');
  conSesion = false;
  expect((await agrupar({ fotos })).status).toBe(401);
  conSesion = true;
  for (const malo of [null, {}, { fotos: 'x' }, { fotos: [{ id: 3 }] }, { fotos: [{}] }]) {
    expect((await agrupar(malo)).status).toBe(400);
  }
  const muchas = Array.from({ length: TOPE_ORGANIZAR + 1 }, (_, i) => ({
    id: `f${i}`,
    description: 'x',
  }));
  expect((await agrupar({ fotos: muchas })).status).toBe(400);
  expect(pedidas).toEqual([]);
});

test('con menos de dos fotos con algo escrito no se llama al modelo', async () => {
  iaQueContesta('{"albumes":[]}');
  const res = await agrupar({ fotos: [fotos[0], fotos[1]] });
  expect(await res.json()).toEqual({ albumes: [] });
  expect(pedidas).toEqual([]);
});

test('lo que no se puede leer se dice, y una lista vacía no es un fallo', async () => {
  iaQueContesta('No he sabido agruparlas.');
  const ilegible = await agrupar({ fotos });
  expect(ilegible.status).toBe(502);
  expect(await ilegible.json()).toEqual({ error: 'ai_unreadable' });
  iaQueContesta('{"albumes": []}');
  expect(await (await agrupar({ fotos })).json()).toEqual({ albumes: [] });
});

test('sin Sytue AI configurada lo dice antes de leer nada', async () => {
  const fuente = await Bun.file(new URL('./agrupar.ts', import.meta.url).pathname).text();
  const sinIA = fuente.indexOf("json({ error: 'ai_not_configured' }, 503)");
  expect(sinIA).toBeGreaterThan(0);
  expect(sinIA).toBeLessThan(fuente.indexOf('await ctx.request.json()'));
});
