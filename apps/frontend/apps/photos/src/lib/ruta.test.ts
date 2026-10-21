import { expect, test } from 'bun:test';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

/*
 * NINGÚN ENLACE INTERNO PUEDE SER RELATIVO.
 *
 * La app se sirve bajo `/photos`, y una ruta relativa se resuelve contra el
 * DIRECTORIO de la página: desde `/photos/` da `/photos/similar`, pero desde
 * `/photos` —sin barra— da `/similar`, un 404 en la raíz del dominio de la
 * organización. Y a `/photos` sin barra se llega por el camino normal: pedirlo
 * redirige al login con `returnTo=%2Fphotos` y al volver se aterriza ahí.
 *
 * No se prueba renderizando: el fallo no depende del componente sino de la URL
 * desde la que se pulsa, y montar cada vista bajo dos URLs distintas costaría
 * más que mirar dónde se escriben.
 */
const PAGINAS = [
  'similar',
  'album',
  'edit',
  'nearby',
  'albums',
  'favorites',
  'trash',
  'duplicates',
  'shared',
];

function fuentes(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory()
      ? fuentes(join(dir, e.name))
      : /\.tsx?$/.test(e.name) && !e.name.includes('.test.')
        ? [join(dir, e.name)]
        : [],
  );
}

test('ningún enlace interno se escribe relativo', () => {
  const malos: string[] = [];
  for (const f of fuentes(new URL('..', import.meta.url).pathname)) {
    readFileSync(f, 'utf8')
      .split('\n')
      .forEach((linea, i) => {
        for (const pag of PAGINAS) {
          // `href={`similar?` o `location.href = `similar?`: sin barra ni prefijo.
          if (new RegExp('(href=\\{|location\\.href = )`' + pag + '[?`]').test(linea)) {
            malos.push(`${f.split('/src/')[1]}:${i + 1}  ${linea.trim().slice(0, 60)}`);
          }
        }
      });
  }
  expect(malos, 'usa `${RUTA}…` (src/lib/ruta.ts), no una ruta relativa').toEqual([]);
});
