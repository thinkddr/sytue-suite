/**
 * El ZIP de la selección pasa por el BFF con la sesión y en streaming. Se lee la
 * fuente, como en `describir.test.ts`, porque importar la ruta arrastra Astro.
 */
import { expect, test } from 'bun:test';

const fuente = await Bun.file(new URL('./zip.ts', import.meta.url).pathname).text();

test('sin sesión no se pide nada a Drive', () => {
  expect(fuente).toContain("if (!sesion) return json({ error: 'unauthenticated' }, 401);");
});

test('pide el ZIP a Drive con el token de la sesión y reenvía el cuerpo sin leerlo', () => {
  expect(fuente).toContain('/api/v1/batch/zip?');
  expect(fuente).toContain('authorization: `Bearer ${sesion.tokens.accessToken}`');
  expect(fuente).toContain('new Response(res.body, { status: 200, headers })');
  expect(fuente).not.toMatch(/res\.(arrayBuffer|blob|text)\(/);
});

test('la descarga es un adjunto que el navegador no puede reinterpretar', () => {
  expect(fuente).toContain("'x-content-type-options': 'nosniff'");
  expect(fuente).toContain("res.headers.get('content-disposition')");
});
