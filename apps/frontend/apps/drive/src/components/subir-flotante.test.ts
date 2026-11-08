// «Subir» flotante en el teléfono (MobileFab), en «Mi unidad». Ejecutar: `bun test`.
//
// Drive no monta componentes en sus pruebas (no tiene DOM de pruebas y no se añade
// una dependencia por esto), así que se vigila el fuente: que el botón exista, que
// llame al MISMO `onUploadClick` que el de la cabecera —no a otro camino de subida— y
// que sin red no se pinte. La pareja nativa es `TestEnElTelefonoSubirFlota`.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';

const fuente = readFileSync(new URL('./FileBrowser.tsx', import.meta.url), 'utf8');

test('«Mi unidad» pinta un MobileFab «Subir» que usa el camino de la cabecera, y no sin red', () => {
  const fab =
    /\{!offline \? \(\s*<MobileFab\s+label=\{t\('files\.upload'\)\}[^]*?onClick=\{onUploadClick\}/;
  expect(fuente).toMatch(fab);
  // Uno solo: el resto de vistas (papelera, recientes…) no suben a ningún sitio.
  expect(fuente.match(/<MobileFab\b/g)?.length).toBe(1);
});
