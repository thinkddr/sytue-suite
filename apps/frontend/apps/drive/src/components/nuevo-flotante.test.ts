// Crear en Drive es el «+» flotante, en todos los anchos, y abre el menú de «Nuevo» con
// «Subir» delante. Ejecutar: `bun test`.
//
// Drive no monta componentes en sus pruebas (no tiene DOM de pruebas y no se añade
// una dependencia por esto), así que se vigila el fuente: que el «+» exista UNA vez, que
// abra el menú (`menu=`) y no haga una sola cosa, que sus entradas usen el MISMO camino
// que tenían los botones de la cabecera, y que esos botones ya no estén: dos controles
// para lo mismo según el ancho es lo que producto pidió quitar (2026-11-25). Que el
// menú cuelgue de verdad del «+» lo prueba `BotonFlotante.test.tsx` en `@sytue/ui`. La
// pareja nativa es `TestNuevoFlota`.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';

const leer = (f: string): string => readFileSync(new URL(f, import.meta.url), 'utf8');

test('«Mi unidad» (la vista que se monta, con la bóveda) crea desde el «+» con menú', () => {
  const fuente = leer('./PrivateFileBrowser.tsx');
  expect(fuente.match(/<BotonFlotante\b/g)?.length).toBe(1);
  const fab =
    /\{section === 'my' \? \(\s*<BotonFlotante\s+label=\{t\('files\.new'\)\}[^]*?menu=\{[^]*?inputRef\.current\?\.click\(\)[^]*?\{t\('files\.upload'\)\}[^]*?setEdit\(\{ kind: 'create' \}\)[^]*?\{t\('files\.newFolder\.title'\)\}/;
  expect(fuente).toMatch(fab);
  // Ni «Subir» ni «Nueva carpeta» como botones de la cabecera: sus rótulos salen una sola
  // vez, en el menú.
  expect(fuente.match(/\{t\('files\.upload'\)\}/g)?.length).toBe(1);
  expect(fuente.match(/\{t\('files\.newFolder\.title'\)\}/g)?.length).toBe(1);
});

test('el explorador clásico también: el menú de «Nuevo», con «Subir» delante, cuelga del «+»', () => {
  const fuente = leer('./FileBrowser.tsx');
  expect(fuente.match(/<BotonFlotante\b/g)?.length).toBe(1);
  const fab =
    /\{!offline \? \(\s*<BotonFlotante\s+label=\{t\('files\.new'\)\}[^]*?menu=\{[^]*?onClick=\{onUploadClick\}[^]*?\{t\('files\.upload'\)\}[^]*?setNewFolderOpen\(true\)[^]*?crearDoc\('document'\)/;
  expect(fuente).toMatch(fab);
  // El desplegable «Nuevo» y el «Subir» primario de la cabecera ya no existen: «Nuevo»
  // solo nombra al «+», y «Subir» solo sale en su menú.
  expect(fuente.match(/t\('files\.new'\)/g)?.length).toBe(1);
  expect(fuente.match(/\{t\('files\.upload'\)\}/g)?.length).toBe(1);
});
