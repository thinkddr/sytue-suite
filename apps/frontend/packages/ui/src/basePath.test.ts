/**
 * El prefijo del HOST manda sobre el del build.
 *
 * La consola se construye UNA vez con `base: '/admin'` y se sirve en dos sitios:
 * `admin.<dominio>/admin` y `platform.<dominio>` en la RAÍZ. Como
 * `import.meta.env.BASE_URL` lo sustituye Vite al construir, en plataforma todos
 * los enlaces salían a `/admin/...`, que allí es un 404 —medido en producción:
 * `/platform` responde 302 y `/admin/platform`, 404—. No se veía porque esa
 * superficie tenía una sola entrada de menú y nadie pulsa la pestaña en la que ya
 * está.
 *
 * Lo que se fija aquí es el contrato entre las dos mitades del arreglo: el
 * servidor publica su prefijo con `basePathScript` y `withBase` lo prefiere. El
 * caso que importa es el VACÍO —plataforma—, porque «publicado y vacío» y «no
 * publicado» llevan a resultados distintos y son fáciles de confundir.
 * Ejecutar: `bun test`.
 */
import { afterEach, describe, expect, test } from 'bun:test';
import { basePathScript, withBase } from './basePath.js';

const publicado = (): string | undefined =>
  (globalThis as { window?: { __sytueBase?: string } }).window?.__sytueBase;

/*
 * SE GUARDA Y SE DEVUELVE lo que hubiera, en vez de borrarlo: cuando todos los
 * ficheros corren en el MISMO proceso —`bun test` desde la raíz del monorepo, en
 * vez de `bun run test`, que entra en cada paquete— el `window` que había era el
 * de happy-dom que registran los tests de componentes, y borrarlo dejaba sin DOM
 * a los que vinieran después. Mismo patrón que `@sytue/auth` en base.test.ts.
 */
const ventanaPrevia = (globalThis as { window?: unknown }).window;

/** Ejecuta el script del servidor como lo haría el navegador. */
function inyectar(base: string): void {
  (globalThis as { window?: unknown }).window = {};
  new Function(basePathScript(base)).call(globalThis);
}

afterEach(() => {
  if (ventanaPrevia === undefined) delete (globalThis as { window?: unknown }).window;
  else (globalThis as { window?: unknown }).window = ventanaPrevia;
});

describe('el prefijo que publica el servidor', () => {
  test('se publica también cuando está vacío (plataforma, servida en la raíz)', () => {
    inyectar('');
    expect(publicado()).toBe('');
  });

  test('se publica con el prefijo cuando la app cuelga de uno', () => {
    inyectar('/admin');
    expect(publicado()).toBe('/admin');
  });

  test('la barra final no cuenta: `/admin/` y `/admin` son el mismo prefijo', () => {
    inyectar('/admin/');
    expect(publicado()).toBe('/admin');
  });

  test('todas las barras finales se eliminan del prefijo', () => {
    inyectar('/admin////');
    expect(publicado()).toBe('/admin');
  });
});

describe('withBase', () => {
  test('sin navegador usa el base del build (render en el servidor)', () => {
    expect(withBase('/admin', '/platform')).toBe('/admin/platform');
  });

  test('EL CASO DEL 404: publicado vacío, el base del build no se aplica', () => {
    inyectar('');
    expect(withBase('/admin', '/platform')).toBe('/platform');
    expect(withBase('/admin', '/conexiones')).toBe('/conexiones');
  });

  test('publicado con prefijo, se aplica aunque el build dijera otra cosa', () => {
    inyectar('/admin');
    expect(withBase('/', '/conexiones')).toBe('/admin/conexiones');
  });

  test('no se prefija dos veces lo que ya viene prefijado', () => {
    inyectar('/admin');
    expect(withBase('/admin', '/admin/users')).toBe('/admin/users');
  });

  test('una ruta relativa se deja como está', () => {
    inyectar('/admin');
    expect(withBase('/admin', 'users')).toBe('users');
  });
});
