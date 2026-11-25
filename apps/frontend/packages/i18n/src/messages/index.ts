/**
 * Los catálogos, indexados por locale. Punto único de importación de los
 * mensajes: quien traduce añade la clave en `en.ts` y el compilador le exige el
 * resto.
 *
 * El servidor (y Bun, en los tests y los scripts) los tiene todos: atiende a la vez a
 * gente de los 26 idiomas y su bundle no viaja a nadie. El NAVEGADOR solo baja uno, el
 * del `<html lang>` que el servidor ya pintó: es el mismo patrón que
 * `loadAppTranslator` con los catálogos de cada app. Importarlos todos costaba ~750 KB
 * (~150 KB en brotli) en cada app y en la cabecera de www.
 *
 * Vite sustituye `import.meta.env.SSR` por una constante, así que en el build de
 * cliente la rama de los 26 muere y con ella sus imports estáticos; cada `import()` de
 * `cargadores` deja un trozo por idioma. Fuera de Vite no hay `import.meta.env.SSR` y
 * por eso se compara con `!== false`: se toma la rama completa.
 *
 * El `await` es de nivel superior a propósito: ningún módulo que traduzca se evalúa
 * antes de que llegue el catálogo, así que la isla hidrata ya traducida (sin un
 * fotograma con las claves en crudo).
 */
import { documentLocale, type Locale } from '../locale.js';
import type { Catalog } from './catalog.js';
import { en } from './en.js';
import { bg } from './bg.js';
import { cs } from './cs.js';
import { da } from './da.js';
import { de } from './de.js';
import { el } from './el.js';
import { es } from './es.js';
import { et } from './et.js';
import { fi } from './fi.js';
import { fr } from './fr.js';
import { ga } from './ga.js';
import { hr } from './hr.js';
import { hu } from './hu.js';
import { is } from './is.js';
import { it } from './it.js';
import { lt } from './lt.js';
import { lv } from './lv.js';
import { mt } from './mt.js';
import { nb } from './nb.js';
import { nl } from './nl.js';
import { pl } from './pl.js';
import { pt } from './pt.js';
import { ro } from './ro.js';
import { sk } from './sk.js';
import { sl } from './sl.js';
import { sv } from './sv.js';

export type { Catalog, MessageKey, PluralForms } from './catalog.js';

type CatalogModule = Partial<Record<Locale, Catalog>>;

/** El catálogo que se sirve en el navegador; `undefined` en el servidor. */
let servido: Catalog | undefined;

/**
 * Baja el catálogo de `locale` y solo ese. El cargador RECHAZA si el trozo no llega y
 * quien lo espera es un `await` de nivel superior: sin `catch`, el módulo se queda sin
 * evaluar y con él toda isla que traduzca. Un reintento cubre el fallo de red
 * pasajero; si tampoco llega, `undefined` y `translate` devuelve la clave —una
 * interfaz sin traducir es un defecto, una pantalla en blanco es una caída—, igual
 * que `loadAppTranslator`.
 */
export async function cargarCatalogo(
  locale: Locale,
  desde: Record<Locale, () => Promise<CatalogModule>>,
): Promise<Catalog | undefined> {
  const cargar = desde[locale];
  const modulo = await cargar().catch(() => cargar().catch(() => undefined));
  return modulo?.[locale];
}

/**
 * Solo en el navegador. Los `import()` van aquí dentro y no en el módulo: en el
 * build del servidor esta función muere con su rama y no queda ningún `import()` de
 * un catálogo que el mismo módulo importa de forma estática (el aviso
 * INEFFECTIVE_DYNAMIC_IMPORT).
 */
async function catalogoDelDocumento(): Promise<Record<Locale, Catalog>> {
  const locale = documentLocale();
  // Un `import()` por idioma: el build de cliente deja cada catálogo en su trozo.
  servido = await cargarCatalogo(locale, {
    en: () => import('./en.js'),
    bg: () => import('./bg.js'),
    cs: () => import('./cs.js'),
    da: () => import('./da.js'),
    de: () => import('./de.js'),
    el: () => import('./el.js'),
    es: () => import('./es.js'),
    et: () => import('./et.js'),
    fi: () => import('./fi.js'),
    fr: () => import('./fr.js'),
    ga: () => import('./ga.js'),
    hr: () => import('./hr.js'),
    hu: () => import('./hu.js'),
    is: () => import('./is.js'),
    it: () => import('./it.js'),
    lt: () => import('./lt.js'),
    lv: () => import('./lv.js'),
    mt: () => import('./mt.js'),
    nb: () => import('./nb.js'),
    nl: () => import('./nl.js'),
    pl: () => import('./pl.js'),
    pt: () => import('./pt.js'),
    ro: () => import('./ro.js'),
    sk: () => import('./sk.js'),
    sl: () => import('./sl.js'),
    sv: () => import('./sv.js'),
  });
  return (servido ? { [locale]: servido } : {}) as Record<Locale, Catalog>;
}

/**
 * Todos los catálogos en el servidor; en el navegador, SOLO el del documento (el
 * tipo describe el del servidor). Para leer el de un idioma en código que también
 * corre en el navegador, `catalogFor`.
 */
export const catalogs: Record<Locale, Catalog> =
  // `as unknown`: fuera de Vite (Bun) el tipo de `import.meta.env` es el de `process.env`.
  (import.meta.env.SSR as unknown) !== false
    ? {
        en,
        bg,
        cs,
        da,
        de,
        el,
        es,
        et,
        fi,
        fr,
        ga,
        hr,
        hu,
        is,
        it,
        lt,
        lv,
        mt,
        nb,
        nl,
        pl,
        pt,
        ro,
        sk,
        sl,
        sv,
      }
    : await catalogoDelDocumento();

/**
 * El catálogo de `locale`, o el que se sirve si el navegador no tiene ese: el
 * documento ya está pintado en un idioma y pedir otro solo puede ser un error (o el
 * instante antes de recargar tras cambiar de idioma). `undefined` solo si el trozo
 * del idioma no llegó.
 */
export function catalogFor(locale: Locale): Catalog | undefined {
  return (catalogs as Partial<Record<Locale, Catalog>>)[locale] ?? servido;
}
