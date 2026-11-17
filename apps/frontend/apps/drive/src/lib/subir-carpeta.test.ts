// «Subir carpeta» con un botón: de `webkitRelativePath` a las entradas de subida.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { admiteSubirCarpeta, entradasDeCarpeta } from './subir-carpeta.js';

/** Un File como los da el input con `webkitdirectory`: con su ruta relativa. */
function fichero(ruta: string): File {
  const f = new File(['x'], ruta.split('/').pop()!);
  Object.defineProperty(f, 'webkitRelativePath', { value: ruta });
  return f;
}

test('cada fichero lleva su subcarpeta, empezando por la carpeta elegida', () => {
  const entradas = entradasDeCarpeta([
    fichero('Proyecto/acta.txt'),
    fichero('Proyecto/planos/a.dwg'),
    fichero('Proyecto/planos/2026/b.dwg'),
  ]);
  expect(entradas.map((e) => [e.file.name, e.dir])).toEqual([
    ['acta.txt', 'Proyecto'],
    ['a.dwg', 'Proyecto/planos'],
    ['b.dwg', 'Proyecto/planos/2026'],
  ]);
});

test('sin ruta relativa el fichero va a la carpeta abierta, y la ruta no sale de ella', () => {
  const suelto = new File(['x'], 'suelto.txt');
  const entradas = entradasDeCarpeta([
    suelto,
    fichero('../Proyecto/./sub//c.txt'),
    fichero('Proyecto\\win\\d.txt'),
  ]);
  expect(entradas.map((e) => e.dir)).toEqual(['', 'Proyecto/sub', 'Proyecto/win']);
});

test('se ofrece donde el input sabe elegir carpetas, y no en iPhone, iPad ni Android', () => {
  const conPropiedad = { webkitdirectory: false };
  const escritorio = {
    userAgent: 'Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0',
    maxTouchPoints: 0,
  };
  expect(admiteSubirCarpeta(conPropiedad, escritorio)).toBe(true);
  expect(admiteSubirCarpeta({}, escritorio)).toBe(false);
  expect(
    admiteSubirCarpeta(conPropiedad, {
      userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Safari/604.1',
      maxTouchPoints: 5,
    }),
  ).toBe(false);
  // Un iPad se anuncia como Mac: lo delata la pantalla táctil.
  expect(
    admiteSubirCarpeta(conPropiedad, {
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15',
      maxTouchPoints: 5,
    }),
  ).toBe(false);
  expect(
    admiteSubirCarpeta(conPropiedad, {
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15',
      maxTouchPoints: 0,
    }),
  ).toBe(true);
  expect(
    admiteSubirCarpeta(conPropiedad, {
      userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 9) Chrome/140.0 Mobile Safari/537.36',
      maxTouchPoints: 5,
    }),
  ).toBe(false);
  expect(admiteSubirCarpeta(null, null)).toBe(false);
});

// Drive no monta componentes en sus pruebas (ver subir-flotante.test.ts): se vigila el
// fuente. El elemento solo se pinta si hay selector de carpetas, el input lleva
// `webkitdirectory` y lo elegido va al MISMO `uploadEntries` que un drop.
test('«Nuevo → Subir carpeta» abre el input de carpetas y sube por el camino del drop', () => {
  const fuente = readFileSync(new URL('../components/FileBrowser.tsx', import.meta.url), 'utf8');
  expect(fuente).toMatch(
    /\{conCarpeta \? \(\s*<MenuItem[^]*?folderInput\.current\?\.click\(\)[^]*?\{t\('files\.upload\.folder'\)\}/,
  );
  expect(fuente).toMatch(
    /\{conCarpeta \? \(\s*<input\s+ref=\{folderInput\}[^]*?webkitdirectory: true[^]*?onFolderSelected/,
  );
  expect(fuente).toMatch(
    /const onFolderSelected[^]*?entradasDeCarpeta\(input\.files[^]*?await uploadEntries\(entries\)/,
  );
});
