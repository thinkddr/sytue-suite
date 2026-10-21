/**
 * El guardián del bundle: ninguna app puede volver a mandar sus cinco catálogos
 * al navegador.
 *
 * `createAppTranslator({ en, es, fr, de, it })` pide los cinco, y pedirlos obliga
 * a importarlos, así que los cinco acababan en el bundle del cliente. Medido en
 * Sytue Admin: el chunk de i18n pesaba 895.861 B (246.097 B gzip) y se descargaba,
 * parseaba y ejecutaba entero en CADA carga para usar una quinta parte. Sumando
 * las ocho apps con catálogo grande eran 1.992.408 B de JS por carga.
 *
 * La forma correcta —y la única que pasa este test— es dejar la llamada de los
 * cinco detrás de `import.meta.env.SSR`, que Vite sustituye por una constante: el
 * servidor los conserva (atiende a los cinco idiomas a la vez y su bundle no
 * viaja a nadie) y el build de cliente los elimina con la rama muerta, dejando un
 * chunk por idioma vía `import.meta.glob`.
 *
 * Se lee el fuente en vez de construir a propósito: construir las trece apps para
 * comprobar esto cuesta minutos, y lo que se quiere atrapar es el fuente.
 */
import { describe, expect, test } from 'bun:test';
import { readdirSync, readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';

const APPS_DIR = join(import.meta.dir, '..', '..', '..', 'apps');

// Cada app, y la ofimática común (`packages/office`), que lleva el catálogo de Docus,
// Calcus, Diapos y CertiPDF: rutas relativas a `apps/`.
const apps = [
  ...readdirSync(APPS_DIR, { withFileTypes: true })
    .filter((d) => d.isDirectory() && existsSync(join(APPS_DIR, d.name, 'src/i18n/index.ts')))
    .map((d) => d.name),
  '../packages/office',
].sort();

describe('catálogos: uno por navegador, no cinco', () => {
  // Si esto falla es que las apps cambiaron de sitio, no que estén bien.
  test('se encuentran las apps con catálogo propio', () => {
    expect(apps.length).toBeGreaterThanOrEqual(13);
  });

  test.each(apps)('%s no manda los cinco catálogos al cliente', (app) => {
    const fuente = readFileSync(join(APPS_DIR, app, 'src/i18n/index.ts'), 'utf8');

    // Sin catálogos importados a pelo no hay nada que mandar (www pasa los textos
    // ya resueltos por props, que es aún mejor). Con ellos, hacen falta las dos
    // piezas: la guarda que mata la rama en el cliente y el glob que la sustituye.
    const importaLosCinco = /import \{ [a-z]{2} \} from '\.\/messages\//.test(fuente);
    if (!importaLosCinco) return;

    expect(fuente).toContain('import.meta.env.SSR');
    expect(fuente).toMatch(/import\.meta\.glob\('\.\/messages\/\*\.ts'\)/);

    // La llamada de los cinco tiene que quedar DENTRO de la rama de servidor: si
    // está suelta, el import estático sobrevive al tree-shaking y vuelven los
    // 895 KB aunque el glob esté puesto.
    //
    // Y la guarda es `!== false`, no `import.meta.env.SSR` a secas: fuera de Vite
    // (estos mismos tests, que corren en Bun) esa propiedad no existe, y con la
    // comparación simple se tomaba la rama del navegador y reventaba al importar.
    const conLosCinco = /createAppTranslator\(\{\s*en(?:,\s*[a-z]{2})+,?\s*\}\)/;
    const guardada =
      /import\.meta\.env\.SSR !== false\s*\?\s*createAppTranslator\(\{\s*en(?:,\s*[a-z]{2})+,?\s*\}\)/;
    if (conLosCinco.test(fuente)) expect(fuente).toMatch(guardada);
  });
});
