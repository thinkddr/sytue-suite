/**
 * Los idiomas se publican app a app: una app puede tener menos catálogos que
 * `LOCALES`. El idioma que le falta se sirve en inglés, en el servidor Y en la
 * isla; antes la isla no encontraba cargador e hidrataba con las claves en crudo.
 */
import { afterEach, describe, expect, test } from 'bun:test';
import { createAppTranslator, loadAppTranslator, type AppCatalog } from './translate.js';

const en = { hola: 'Hello', ficheros: { one: '{count} file', other: '{count} files' } };
const es: AppCatalog<typeof en> = {
  hola: 'Hola',
  ficheros: { one: '{count} archivo', other: '{count} archivos' },
};

// Se devuelve lo que hubiera en vez de borrarlo: con `bun test` desde la raíz
// del monorepo todos los ficheros comparten proceso, y el `document` que había
// era el de happy-dom que registran los tests de componentes.
const documentoPrevio = (globalThis as { document?: unknown }).document;

describe('app con catálogos a medias', () => {
  afterEach(() => {
    if (documentoPrevio === undefined) delete (globalThis as { document?: unknown }).document;
    else (globalThis as { document?: unknown }).document = documentoPrevio;
  });

  test('el servidor sirve en inglés el idioma que la app aún no tiene', () => {
    const t = createAppTranslator({ en, es })('pl');
    expect(t('hola')).toBe('Hello');
    // «3» es `few` en polaco y el inglés no tiene esa forma: cae a `other`.
    expect(t('ficheros', { count: 3 })).toBe('3 files');
  });

  test('la isla baja el catálogo inglés en vez de pintar claves', async () => {
    (globalThis as { document?: unknown }).document = { documentElement: { lang: 'pl' } };
    const pedidos: string[] = [];
    const cargador = (cc: string, catalogo: object) => () => {
      pedidos.push(cc);
      return Promise.resolve({ [cc]: catalogo });
    };
    const useT = await loadAppTranslator<typeof en>({
      './messages/en.ts': cargador('en', en),
      './messages/es.ts': cargador('es', es),
    });
    expect(useT('pl')('hola')).toBe('Hello');
    expect(pedidos).toEqual(['en']);
  });
});
