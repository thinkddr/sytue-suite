import { expect, test } from 'bun:test';
import { NotesClient } from './index.js';

function client(fetchImpl: typeof fetch): NotesClient {
  return new NotesClient({
    baseUrl: 'https://notes.invalid',
    auth: { getAccessToken: () => 'fixture-token' },
    fetchImpl,
  });
}

// Un ZIP de Obsidian es binario: si el cuerpo pasara por una cadena, los bytes
// no UTF-8 se cambiarían por U+FFFD y el servicio recibiría un ZIP roto. Y el
// nombre del fichero viaja, porque es lo que titula el cuaderno.
test('la importación manda los bytes intactos y el nombre del fichero', async () => {
  const zip = new Uint8Array([0x50, 0x4b, 0x03, 0x04, 0xff, 0x00, 0x9c]);
  const vistos: { url: string; cuerpo: Uint8Array }[] = [];
  const sdk = client(async (url, init) => {
    vistos.push({
      url: String(url),
      cuerpo: new Uint8Array(await new Response(init?.body ?? null).arrayBuffer()),
    });
    return Response.json({ notebook: {}, notas: [], importadas: 0, descartadas: 0 });
  });
  await sdk.previewNotebook(zip.buffer, 'Bóveda.zip');
  await sdk.importNotebook(zip.buffer, 'Bóveda.zip');
  for (const v of vistos) {
    expect(new URL(v.url).searchParams.get('name')).toBe('Bóveda.zip');
    expect([...v.cuerpo]).toEqual([...zip]);
  }
  expect(new URL(vistos[0]!.url).pathname).toBe('/api/v1/notebooks/preview');
  expect(new URL(vistos[1]!.url).pathname).toBe('/api/v1/notebooks/import');
});
