/**
 * Los assets del <head> se unen al prefijo con `withBase`, nunca por
 * concatenación.
 *
 * `import.meta.env.BASE_URL` llega SIN barra final —`/mail`— y la forma
 * `${BASE_URL}favicon.svg` produce `/mailfavicon.svg`. En producción eso era un
 * 404 medido con el navegador: ninguna de las once superficies tenía icono en
 * la pestaña, y como Sytue ID sí lo tenía —es Go y escribe `/favicon.svg` a
 * mano— el fallo parecía del IdP y no de las apps.
 *
 * Lo que se fija aquí son las dos mitades: que `withBase` una bien en los dos
 * casos que existen en producción (prefijo con y sin barra, y prefijo VACÍO para
 * `platform.<dominio>`, que se sirve en la raíz), y que ningún `.astro` vuelva a
 * concatenar. La segunda es la que importa: el primer fallo lo arregló un commit
 * en trece ficheros a la vez, y el decimocuarto lo escribirá alguien mañana
 * copiando el patrón que tenga más a mano.
 */
import { describe, expect, it } from 'bun:test';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { withBase } from '../basePath.js';

const APPS = fileURLToPath(new URL('../../../../apps/', import.meta.url));
/** Las vistas de la ofimática se pintan en las apps que las montan (`packages/office`). */
const OFICINA = fileURLToPath(new URL('../../../office/src/', import.meta.url));

/** Todos los `.astro` de todas las apps, sin listarlos a mano. */
function astros(dir: string, acc: string[] = []): string[] {
  for (const nombre of readdirSync(dir)) {
    if (nombre === 'node_modules' || nombre === 'dist' || nombre === '.astro') continue;
    const ruta = join(dir, nombre);
    if (statSync(ruta).isDirectory()) astros(ruta, acc);
    else if (nombre.endsWith('.astro')) acc.push(ruta);
  }
  return acc;
}

describe('assets del <head> y el prefijo de la app', () => {
  it('withBase une con barra venga como venga el prefijo', () => {
    expect(withBase('/mail', '/favicon.svg')).toBe('/mail/favicon.svg');
    expect(withBase('/mail/', '/favicon.svg')).toBe('/mail/favicon.svg');
    expect(withBase('/one', '/icons/apple-touch-icon-180.png')).toBe(
      '/one/icons/apple-touch-icon-180.png',
    );
  });

  // `platform.<dominio>` sirve la consola en la RAÍZ y publica prefijo vacío:
  // ahí el icono tiene que quedarse en `/favicon.svg`, no en `/admin/favicon.svg`.
  it('con prefijo vacío el asset se queda en la raíz', () => {
    expect(withBase('', '/favicon.svg')).toBe('/favicon.svg');
  });

  it('ningún .astro concatena BASE_URL con un fichero', () => {
    const malos: string[] = [];
    for (const ruta of [...astros(APPS), ...astros(OFICINA)]) {
      const src = readFileSync(ruta, 'utf8');
      const lineas = src.split('\n');
      lineas.forEach((l, i) => {
        if (/import\.meta\.env\.BASE_URL\}[A-Za-z]/.test(l)) {
          malos.push(`${ruta.replace(APPS, '')}:${i + 1}`);
        }
      });
    }
    expect(malos).toEqual([]);
  });
});
