/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Mostrar en su carpeta» y abrir una carpeta desde Recientes, Destacados y la búsqueda.
 * Ejecutar: `bun test`. Lo puro va por sus funciones; el cableado (Shell → explorador,
 * el menú y la carpeta pulsable) se vigila en el fuente, como en `vaciar-papelera.test.ts`,
 * porque Drive no monta componentes en sus pruebas. La pareja nativa son los
 * `TestUbicacion*` y `TestLaCarpetaPlana*` de `apps/fyne/apps/drive`.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  aMarcar,
  carpetaEnCache,
  carpetaPadre,
  pendienteDeLaUrl,
  urlDeListado,
  urlDeUbicacion,
} from './ubicacion';

test('la carpeta de un elemento sale de su ruta', () => {
  expect(carpetaPadre('/Actas/2026/acta.pdf')).toBe('/Actas/2026');
  expect(carpetaPadre('/acta.pdf')).toBe('/');
  expect(carpetaPadre('/Actas/2026/')).toBe('/Actas');
  expect(carpetaPadre('/Actas')).toBe('/');
  expect(carpetaPadre('/')).toBe('/');
});

test('la URL abre la carpeta, marca el elemento y no arrastra el visor ni la sección', () => {
  const desde = 'https://acme.sytue.eu/drive/?section=recent&preview=%2Fotra.pdf&lang=es';
  const u = new URL(urlDeUbicacion(desde, '/Actas/2026', '/Actas/2026/acta.pdf'));
  expect(u.pathname).toBe('/drive/');
  expect(u.searchParams.get('path')).toBe('/Actas/2026');
  expect(u.searchParams.get('select')).toBe('/Actas/2026/acta.pdf');
  // `?preview=` abriría el visor de OTRO fichero al llegar y `?section=` mandaría a
  // Recientes otra vez.
  expect(u.searchParams.has('preview')).toBe(false);
  expect(u.searchParams.has('section')).toBe(false);
  expect(u.searchParams.get('lang')).toBe('es');
});

test('abrir una carpeta sin marcar nada: la raíz no lleva ?path= y no queda un ?select= viejo', () => {
  const desde = 'https://acme.sytue.eu/drive/?path=%2FViejo&select=%2FViejo%2Fa.txt';
  const raiz = new URL(urlDeUbicacion(desde, '/'));
  expect(raiz.searchParams.has('path')).toBe(false);
  expect(raiz.searchParams.has('select')).toBe(false);
  const carpeta = new URL(urlDeUbicacion(desde, '/Proyectos'));
  expect(carpeta.searchParams.get('path')).toBe('/Proyectos');
  expect(carpeta.searchParams.has('select')).toBe(false);
});

test('lo que marcar se lee de la URL con la que se llega', () => {
  expect(pendienteDeLaUrl('?path=%2FActas&select=%2FActas%2Facta.pdf')).toEqual({
    carpeta: '/Actas',
    ruta: '/Actas/acta.pdf',
  });
  expect(pendienteDeLaUrl('?select=%2Facta.pdf')).toEqual({ carpeta: '/', ruta: '/acta.pdf' });
  expect(pendienteDeLaUrl('?path=%2FActas&preview=%2FActas%2Fa.pdf')).toBeNull();
});

test('se marca al llegar SU carpeta, se espera a la lista nueva y se olvida al navegar', () => {
  const pendiente = { carpeta: '/Actas', ruta: '/Actas/acta.pdf' };
  const vieja = [{ path: '/Otra/x.pdf' }];
  const nueva = [{ path: '/Actas/Anexos' }, { path: '/Actas/acta.pdf' }];
  expect(aMarcar(null, pendiente, '/Actas')).toBe('esperar');
  // En el pintado en que cambia la ruta aún se ve la lista de antes.
  expect(aMarcar(vieja, pendiente, '/Actas')).toBe('esperar');
  expect(aMarcar(nueva, pendiente, '/Actas')).toEqual({ item: nueva[1]!, indice: 1 });
  expect(aMarcar(nueva, pendiente, '/Otra')).toBe('olvidar');
});

test('si la lista ya es la de su carpeta y no está, falta (otra página); vacía, se espera', () => {
  const pendiente = { carpeta: '/Actas', ruta: '/Actas/zeta.pdf' };
  const primera = [{ path: '/Actas/Anexos' }, { path: '/Actas/acta.pdf' }];
  expect(aMarcar(primera, pendiente, '/Actas')).toBe('falta');
  // La raíz: sus hijos cuelgan de «/».
  expect(aMarcar([{ path: '/b.txt' }], { carpeta: '/', ruta: '/z.txt' }, '/')).toBe('falta');
  // Una lista vacía no dice de qué carpeta es: puede ser la anterior.
  expect(aMarcar([], pendiente, '/Actas')).toBe('esperar');
});

test('sin red, la carpeta se ofrece solo si su listado está guardado', async () => {
  const guardadas: string[] = [];
  const almacen = {
    match: async (req: RequestInfo | URL) =>
      guardadas.includes(String(req)) ? new Response('{}') : undefined,
  } as Pick<CacheStorage, 'match'>;
  const base = 'https://acme.sytue.eu/drive/?section=recent';
  expect(await carpetaEnCache('/Actas', almacen, base)).toBe(false);
  // La clave es la MISMA petición que hace `drive.list` al abrir la carpeta.
  guardadas.push(new URL(urlDeListado('/Actas'), base).toString());
  expect(await carpetaEnCache('/Actas', almacen, base)).toBe(true);
  expect(await carpetaEnCache('/Otra', almacen, base)).toBe(false);
  // Sin Cache Storage, o si falla, no se promete nada.
  expect(await carpetaEnCache('/Actas', undefined, base)).toBe(false);
  const roto = {
    match: () => Promise.reject(new Error('SecurityError')),
  } as Pick<CacheStorage, 'match'>;
  expect(await carpetaEnCache('/Actas', roto, base)).toBe(false);
});

// --- Cableado (fuente) ---------------------------------------------------------

const explorador = readFileSync(new URL('../components/FileBrowser.tsx', import.meta.url), 'utf8');
const privateExplorador = readFileSync(
  new URL('../components/PrivateFileBrowser.tsx', import.meta.url),
  'utf8',
);
const shell = readFileSync(new URL('../components/Shell.tsx', import.meta.url), 'utf8');
const plana = explorador.slice(explorador.indexOf('export function ResourceListView'));
const menu = explorador.slice(explorador.indexOf('function ActionsMenu('));

test('la clave de la caché lleva el prefijo de la app, como la petición que guardó el worker', () => {
  const w = globalThis as { window?: unknown };
  const antes = w.window;
  w.window = { __sytueBase: '/drive' };
  try {
    expect(urlDeListado('/Actas')).toBe('/drive/api/files?path=%2FActas&limit=200');
  } finally {
    w.window = antes;
  }
});

test('el listado de la carpeta usa la misma URL que mira la caché', () => {
  expect(explorador).toMatch(
    /list: \(path: string, cursor\?: string, orden\?: Orden\) =>\s*bffJson<[^>]+>\(urlDeListado\(path, cursor, orden\)\)/,
  );
});

test('en Recientes y Destacados la carpeta es un botón que la abre en «Mi unidad»', () => {
  expect(plana).toMatch(
    /item\.isDirectory \? onOpenFolder\?\.\(item\.path\) : void handleOpen\(item\)/,
  );
  expect(plana).toMatch(/<ActionsMenu[^>]*onShowInFolder=\{onShowInFolder\}/);
  expect(privateExplorador).toMatch(
    /if \(item\.isDirectory\) \{\s*remember\(item\);\s*if \(section !== 'my'\) \{[^]*?url\.searchParams\.set\('section', 'my'\);\s*window\.history\.replaceState\(null, '', url\);\s*if \(onOpenFolder\) onOpenFolder\(\);/,
  );
  // Shell monta PrivateFileBrowser con el cambio a «Mi unidad».
  expect(shell).toMatch(
    /<PrivateFileBrowser[\s\S]*?onOpenFolder=\{\(\) => \{\s*setCloud\(null\);\s*setSection\('my'\);\s*\}\}/,
  );
});

test('el menú lo ofrece sin red solo con la carpeta guardada', () => {
  expect(menu).toMatch(/carpetaEnCache\(carpetaPadre\(item\.path\)\)/);
  expect(menu).toMatch(/onShowInFolder && \(!offline \|\| carpetaGuardada\) \? \(/);
  expect(menu).toMatch(/t\('action\.showInFolder'\)/);
});

test('en la búsqueda se vacía la consulta, se va a la carpeta y se marca al llegar', () => {
  expect(explorador).toMatch(
    /const onShowInFolder = \(item: DriveResource\) => \{[^]*?wantSelect\.current = \{ carpeta, ruta: item\.path \};\s*setQuery\(''\);\s*goTo\(carpeta\);/,
  );
  expect(
    explorador.match(/onShowInFolder=\{searchMode \? onShowInFolder : undefined\}/g),
  ).toHaveLength(2);
  // Marcar va DETRÁS del efecto que vacía la selección al salir de la búsqueda.
  const vacia = explorador.indexOf('}, [query, runSearch]);');
  const marca = explorador.indexOf('const r = aMarcar(partes.enOrden, want, path);');
  expect(vacia).toBeGreaterThan(0);
  expect(marca).toBeGreaterThan(vacia);
  // Si no está en lo cargado, se piden más páginas en vez de marcarlo de golpe luego.
  expect(explorador).toMatch(
    /if \(r === 'falta' && nextCursor !== null\) \{\s*void loadMore\(\);\s*return;\s*\}/,
  );
  expect(explorador).toMatch(
    /listaRef\.current\?\.scrollToIndex\(enLista, \{ align: 'center' \}\)/,
  );
  // `?select=` es de un solo uso, como `?preview=`.
  expect(explorador).toMatch(
    /u\.searchParams\.delete\('preview'\);\s*u\.searchParams\.delete\('select'\);/,
  );
});
