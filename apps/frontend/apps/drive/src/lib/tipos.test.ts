// «Filtrar por tipo», la ruta entera y el «anterior / siguiente» de la vista previa del
// Drive de la bóveda. Ejecutar: `bun test`.
//
// Drive no monta componentes en sus pruebas (ver `nuevo-flotante.test.ts`): la lógica se
// prueba aquí y, de la vista, se vigila el fuente.
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { FILTROS_TIPO, filtrarPorTipo, migasDe, tipoDe, vecinoDe } from './tipos.js';

const el = (name: string, extra: { isDirectory?: boolean; contentType?: string } = {}) => ({
  path: `/a/${name}`,
  name,
  isDirectory: extra.isDirectory ?? false,
  ...(extra.contentType ? { contentType: extra.contentType } : {}),
});

describe('tipoDe', () => {
  test('por el contentType y, si no lo trae, por la extensión', () => {
    expect(tipoDe(el('Facturas', { isDirectory: true }))).toBe('carpetas');
    expect(tipoDe(el('acta', { contentType: 'application/pdf' }))).toBe('pdf');
    expect(tipoDe(el('ACTA.PDF'))).toBe('pdf');
    expect(tipoDe(el('foto.heic'))).toBe('imagenes');
    expect(tipoDe(el('x', { contentType: 'image/png' }))).toBe('imagenes');
    expect(tipoDe(el('reunion.webm'))).toBe('media');
    expect(tipoDe(el('nota', { contentType: 'audio/ogg' }))).toBe('media');
    expect(tipoDe(el('informe.docx'))).toBe('documentos');
    expect(tipoDe(el('cuentas.ods'))).toBe('documentos');
    expect(tipoDe(el('x', { contentType: 'text/plain' }))).toBe('documentos');
    expect(tipoDe(el('copia.zip'))).toBe('otros');
  });

  test('cada filtro deja lo suyo, en el orden de la carpeta; «todo» no quita nada', () => {
    const items = [
      el('C', { isDirectory: true }),
      el('b.pdf'),
      el('a.png'),
      el('z.zip'),
      el('c.pdf'),
    ];
    expect(filtrarPorTipo(items, 'todo')).toEqual(items);
    expect(filtrarPorTipo(items, 'pdf').map((i) => i.name)).toEqual(['b.pdf', 'c.pdf']);
    expect(filtrarPorTipo(items, 'carpetas').map((i) => i.name)).toEqual(['C']);
    expect(filtrarPorTipo(items, 'media')).toEqual([]);
    expect(FILTROS_TIPO[0]).toBe('todo');
  });
});

test('vecinoDe salta las carpetas y se para en los extremos', () => {
  const items = [el('a.png'), el('C', { isDirectory: true }), el('b.png'), el('c.png')];
  const [a, , b, c] = items;
  expect(vecinoDe(items, a!, 1)?.name).toBe('b.png');
  expect(vecinoDe(items, b!, -1)?.name).toBe('a.png');
  expect(vecinoDe(items, a!, -1)).toBeNull();
  expect(vecinoDe(items, c!, 1)).toBeNull();
  // Un fichero que ya no está en la lista (filtrada, o borrado) no tiene vecinos.
  expect(vecinoDe(items, el('fuera.png'), 1)).toBeNull();
});

test('migasDe da cada carpeta de la ruta, de la raíz al final', () => {
  expect(migasDe('/')).toEqual([]);
  expect(migasDe('/a/b c/d')).toEqual([
    { nombre: 'a', ruta: '/a' },
    { nombre: 'b c', ruta: '/a/b c' },
    { nombre: 'd', ruta: '/a/b c/d' },
  ]);
});

test('el Drive de la bóveda usa todo esto, y una búsqueda vacía no dice «carpeta vacía»', () => {
  const fuente = readFileSync(
    new URL('../components/PrivateFileBrowser.tsx', import.meta.url),
    'utf8',
  );
  // Las migas de siempre, con «Mi unidad» y cada carpeta de la ruta.
  expect(fuente).toMatch(/<Migas\s+label=\{t\('files\.breadcrumb'\)\}/);
  expect(fuente).toContain('...migasDe(path).map(');
  // La búsqueda sin resultados va ANTES que la carpeta vacía, con su botón para salir.
  const busqueda = fuente.indexOf("t('files.search.emptyTitle'");
  const carpeta = fuente.indexOf("'files.empty.title'");
  expect(busqueda).toBeGreaterThan(0);
  expect(busqueda).toBeLessThan(carpeta);
  expect(fuente).toContain("{t('files.search.clear')}");
  // La lista pinta lo filtrado, y la vista previa pasa entre esos mismos ficheros.
  expect(fuente).toContain('items={visibles ?? items}');
  expect(fuente).toContain('anterior={vecinoDe(visibles ?? [], preview, -1)}');
  // «Mostrar en su carpeta» fuera de la carpeta: Recientes, Destacados y la búsqueda.
  expect(fuente).toMatch(
    /onShowInFolder=\{\s*section === 'recent' \|\| section === 'favorites' \|\| search\.trim\(\)\s*\? showInFolder/,
  );
});
