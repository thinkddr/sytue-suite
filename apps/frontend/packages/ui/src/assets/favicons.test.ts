/**
 * Cada superficie tiene que llevar SU icono, y en los DOS formatos.
 *
 * El `.svg` lo declara el HTML y es lo que ve un navegador que parsea la página.
 * Pero cuando algo pide `/favicon.ico` a pelo —previsualización de un enlace,
 * lector de feeds, atajo del sistema, anclaje a la barra de tareas de Windows—
 * no hay HTML que leer: sin fichero es un 404 y un icono en blanco. Windows
 * además NO acepta un SVG para anclar, así que ahí el `.ico` no es sustituible
 * por una redirección.
 *
 * Lo que este test impide es la desincronización silenciosa, que ya ha pasado
 * dos veces: los favicons escritos a mano acabaron con siete colores fuera de
 * paleta, y el conmutador de aplicaciones se quedó con la familia anterior
 * mientras la pestaña enseñaba la nueva. Un icono desfasado no rompe nada —la
 * app funciona— así que solo lo caza un gate.
 *
 * Los ficheros los genera `scripts/sync-branding.sh` desde `branding/marks/`.
 * Aquí NO se comprueba el píxel: se comprueba que existan, que sean de verdad lo
 * que dicen ser, y que ninguna superficie comparta icono con otra —que es la
 * forma en que se manifiesta un copiar-pegar olvidado—.
 */
import { describe, expect, it } from 'bun:test';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const APPS = fileURLToPath(new URL('../../../../apps/', import.meta.url));

/** Superficies con marca propia. `www` es el sitio de marca, no un producto. */
const SUPERFICIES = [
  'portal',
  'console',
  'cloud-dashboard',
  'mail',
  'drive',
  'calendar',
  'contacts',
  // La ofimática: una app por producto desde el 2026-10-12 (ADR 0076).
  'docus',
  'calcus',
  'diapos',
  'certipdf',
  'notes',
  'rooms',
  'photos',
  'erp',
  'cast',
  'pages',
  'bases',
  'account',
  'www',
];

const ruta = (app: string, ext: string) => `${APPS}${app}/public/favicon.${ext}`;

describe('favicons de las superficies', () => {
  it('cada superficie trae su .svg y su .ico', () => {
    const faltan = SUPERFICIES.flatMap((app) =>
      ['svg', 'ico'].filter((ext) => !existsSync(ruta(app, ext))).map((ext) => `${app}.${ext}`),
    );
    expect(faltan).toEqual([]);
  });

  // Un `.ico` que en realidad es un PNG o un SVG renombrado se sirve igual y
  // falla solo en el consumidor que lo pedía por ser `.ico`. La cabecera lo
  // delata: los cuatro primeros bytes de un ICO son 00 00 01 00.
  it('los .ico son iconos de verdad, no otro formato renombrado', () => {
    for (const app of SUPERFICIES) {
      const buf = readFileSync(ruta(app, 'ico'));
      expect(`${app}:${buf[0]}${buf[1]}${buf[2]}${buf[3]}`).toBe(`${app}:0010`);
      // Y multi-resolución: con un solo tamaño el escalado lo hace el sistema
      // operativo y se nota justo donde más se mira, en la pestaña a 16 px.
      expect(`${app}: ${buf.readUInt16LE(4)} tamaños`).toBe(`${app}: 4 tamaños`);
    }
  });

  // Safari e iOS no rasterizan SVG: para Favoritos, la pestaña de reserva y
  // «añadir a inicio» piden un PNG (`apple-touch-icon`). Rooms y Cloud lo
  // declaraban como SVG y ocho apps no lo declaraban, así que Safari caía al
  // `/apple-touch-icon.png` de la raíz del host, que tampoco existía. Se exige
  // el fichero, que sea PNG de verdad (cabecera 89 50 4E 47) y que mida 180×180,
  // que es lo que iOS espera y lo que genera scripts/sync-branding.sh.
  // Safari elige el icono MÁS GRANDE declarado y trata mejor el PNG que el ICO:
  // el PNG de 64 px con alfa es el que enseña en la pestaña (2026-09-09).
  it('cada superficie trae su favicon-64.png de 64×64', () => {
    for (const app of SUPERFICIES) {
      const f = `${APPS}${app}/public/favicon-64.png`;
      expect(`${app}: ${existsSync(f)}`).toBe(`${app}: true`);
      const buf = readFileSync(f);
      expect(`${app}: ${buf.subarray(0, 4).toString('hex')}`).toBe(`${app}: 89504e47`);
      expect(`${app}: ${buf.readUInt32BE(16)}×${buf.readUInt32BE(20)}`).toBe(`${app}: 64×64`);
    }
  });

  it('cada superficie trae su apple-touch-icon.png de 180×180', () => {
    for (const app of SUPERFICIES) {
      const f = `${APPS}${app}/public/apple-touch-icon.png`;
      expect(`${app}: ${existsSync(f)}`).toBe(`${app}: true`);
      const buf = readFileSync(f);
      expect(`${app}: ${buf.subarray(0, 4).toString('hex')}`).toBe(`${app}: 89504e47`);
      // IHDR: anchura y altura en big-endian en los bytes 16-23.
      expect(`${app}: ${buf.readUInt32BE(16)}×${buf.readUInt32BE(20)}`).toBe(`${app}: 180×180`);
    }
  });

  // Si dos superficies comparten icono es que alguien copió el fichero en vez de
  // regenerarlo desde su marca. No da error en ninguna parte: simplemente dos
  // productos distintos salen iguales en la pestaña.
  it('ninguna superficie comparte icono con otra', () => {
    const porHash = new Map<string, string[]>();
    for (const app of SUPERFICIES) {
      const h = createHash('sha256')
        .update(readFileSync(ruta(app, 'ico')))
        .digest('hex');
      porHash.set(h, [...(porHash.get(h) ?? []), app]);
    }
    const repetidos = [...porHash.values()].filter((apps) => apps.length > 1);
    expect(repetidos).toEqual([]);
  });
});
