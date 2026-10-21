// Check de que TODAS las superficies aceptan GET en /auth/logout.
//
// Cerrar sesion es siempre un GET: el menu de cuenta NAVEGA a /auth/logout y el
// barrido de cookies de Sytue ID llega como subrecurso. Las apps que solo
// exportaban POST respondian 404 y el cierre de sesion no hacia nada: ni borraba
// su cookie ni llegaba al end_session. Divergio en silencio porque la ruta vive
// en diez ficheros; este check es el unico sitio donde se ve el conjunto.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const APPS_DIR = join(import.meta.dir, '../../../../apps');

test('cada app con BFF exporta GET y POST en /auth/logout', () => {
  const rutas = readdirSync(APPS_DIR, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => join(APPS_DIR, e.name, 'src/pages/auth/logout.ts'))
    .filter((p) => {
      try {
        readFileSync(p);
        return true;
      } catch {
        return false; // app sin sesion (www)
      }
    });
  // Si el glob se rompe, el test pasaria en vacio: exigir que encuentre las diez.
  expect(rutas.length).toBeGreaterThanOrEqual(9);
  for (const ruta of rutas) {
    const src = readFileSync(ruta, 'utf8');
    // Las dos formas valen: la ruta ya no implementa nada, reexporta el handler
    // comun (`export { logout as GET, logout as POST } from './_rutas.js'`). Lo
    // que importa sigue siendo lo mismo: que el verbo salga del modulo.
    expect(src).toMatch(/export const GET\b|\bas GET\b/);
    expect(src).toMatch(/export const POST\b|\bas POST\b/);
  }
});
