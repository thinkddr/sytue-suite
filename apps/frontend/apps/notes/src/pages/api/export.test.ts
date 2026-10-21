/**
 * Los formatos de exportación viven en TRES sitios —el servicio Go, el SDK y la
 * lista que pinta el menú— y nada obliga a que coincidan. La forma en que esto se
 * rompe no es un error: es una entrada de menú que descarga un 400, o un formato
 * que el servicio sabe generar y que nadie puede pedir.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';
import { FORMATOS_EXPORT, urlExport } from '../../client/notesApi.js';

// Rutas desde el FICHERO y no desde el directorio de trabajo: con `bun test`
// lanzado en la raíz de apps/frontend —en vez de `bun run test`, que entra en
// cada paquete— estas lecturas daban ENOENT y el test moría sin comparar nada.
const APP = join(import.meta.dir, '../../..');
const FRONTEND = join(APP, '../..');
const RAIZ = join(APP, '../../../..');

/** Lo que acepta `FormatoDe` en services/notes/internal/domain/exportar.go. */
async function formatosDelServicio(): Promise<Set<string>> {
  const fuente = await Bun.file(join(RAIZ, 'services/notes/internal/domain/exportar.go')).text();
  const bloque = fuente.slice(fuente.indexOf('func FormatoDe'));
  return new Set(
    [...bloque.slice(0, bloque.indexOf('\n}')).matchAll(/case ([^:]+):/g)].flatMap((m) =>
      [...(m[1] ?? '').matchAll(/"([a-z]*)"/g)].flatMap((c) => (c[1] ? [c[1]] : [])),
    ),
  );
}

test('todo lo que ofrece el menú lo acepta el servicio', async () => {
  const acepta = await formatosDelServicio();
  for (const f of FORMATOS_EXPORT) {
    expect(acepta.has(f.id)).toBe(true);
  }
});

test('el SDK declara los mismos formatos que el servicio', async () => {
  const acepta = await formatosDelServicio();
  const sdk = await Bun.file(join(FRONTEND, 'packages/api-clients/src/notes/index.ts')).text();
  const linea = sdk.slice(sdk.indexOf('static readonly formatosExport'));
  const declarados = [...linea.slice(0, linea.indexOf(';')).matchAll(/'([a-z]+)'/g)].flatMap((m) =>
    m[1] ? [m[1]] : [],
  );
  for (const f of declarados) {
    expect(acepta.has(f)).toBe(true);
  }
  // `text` y `htm` son alias que el servicio tolera al escribirlos a mano; no se
  // ofrecen. Lo que no puede pasar es lo contrario: un formato generable que el
  // SDK no sepa pedir.
  for (const f of acepta) {
    if (f === '' || f === 'text' || f === 'htm') continue;
    expect(declarados).toContain(f);
  }
});

test('el HTML se descarga y nunca se abre en la pestaña', () => {
  // Lleva contenido del cuaderno: servirlo `inline` en el origen de la app le
  // daría el mismo origen que la sesión. El servicio también lo impide, pero un
  // menú que ofrece «ver» un HTML promete algo que acabaría en descarga.
  expect(FORMATOS_EXPORT.find((f) => f.id === 'html')?.ver).toBe(false);
  expect(FORMATOS_EXPORT.find((f) => f.id === 'pdf')?.ver).toBe(true);
});

test('la URL lleva el formato y solo pide vista cuando se pide', () => {
  expect(urlExport('abc', 'pdf')).toContain('format=pdf');
  expect(urlExport('abc', 'pdf')).not.toContain('view=');
  expect(urlExport('abc', 'pdf', true)).toContain('view=1');
  // El identificador va escapado: viene de la API, pero es lo que separa una
  // ruta de una inyección de parámetros en la query de al lado.
  expect(urlExport('a/b?c=1', 'md')).toContain('a%2Fb%3Fc%3D1');
});
