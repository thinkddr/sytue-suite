/**
 * El navegador baja UN catálogo compartido, el del documento (`messages/index.ts`). En
 * Bun se toma la rama del servidor (los 26), así que aquí se prueba la mecánica que usa
 * el navegador: qué cargador se llama, el reintento y la red sin catálogo.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { LOCALES, type Locale } from './locale.js';
import { catalogFor, catalogs, cargarCatalogo, type Catalog } from './messages/index.js';
import { createAppTranslator, translate } from './translate.js';

type Loaders = Record<Locale, () => Promise<Partial<Record<Locale, Catalog>>>>;

function contados(fallos = 0): { loaders: Loaders; llamadas: Locale[] } {
  const llamadas: Locale[] = [];
  let pendientes = fallos;
  const loaders = Object.fromEntries(
    LOCALES.map((locale) => [
      locale,
      async () => {
        llamadas.push(locale);
        if (pendientes-- > 0) throw new TypeError('Failed to fetch dynamically imported module');
        return { [locale]: catalogs[locale] };
      },
    ]),
  ) as Loaders;
  return { loaders, llamadas };
}

test('every locale has its own loader, and each is a separate import()', () => {
  const fuente = readFileSync(new URL('./messages/index.ts', import.meta.url), 'utf8');
  for (const locale of LOCALES)
    expect(fuente).toContain(`${locale}: () => import('./${locale}.js')`);
});

test('only the document locale is fetched', async () => {
  const { loaders, llamadas } = contados();
  expect(await cargarCatalogo('es', loaders)).toBe(catalogs.es);
  expect(llamadas).toEqual(['es']);
});

test('a chunk that fails once is retried once', async () => {
  const { loaders, llamadas } = contados(1);
  expect(await cargarCatalogo('fr', loaders)).toBe(catalogs.fr);
  expect(llamadas).toEqual(['fr', 'fr']);
});

test('a chunk that never arrives leaves no catalog instead of throwing', async () => {
  const { loaders, llamadas } = contados(2);
  expect(await cargarCatalogo('de', loaders)).toBeUndefined();
  expect(llamadas).toEqual(['de', 'de']);
});

test('each locale module exports the catalog the server uses under its own name', async () => {
  for (const locale of LOCALES)
    expect(((await import(`./messages/${locale}.ts`)) as Record<string, unknown>)[locale]).toBe(
      catalogs[locale],
    );
});

test('server side, every locale translates from its own catalog', () => {
  for (const locale of LOCALES) expect(catalogFor(locale)).toBe(catalogs[locale]);
  expect(translate('es', 'action.retry')).toBe(catalogs.es['action.retry'] as string);
});

test('an app catalog still falls back to the shared chrome', () => {
  const t = createAppTranslator({ en: { 'app.only': 'Only here' } })('es');
  expect(t('app.only')).toBe('Only here');
  expect(t('action.retry')).toBe(catalogs.es['action.retry'] as string);
  expect(t('not.a.key' as 'app.only')).toBe('not.a.key');
});
