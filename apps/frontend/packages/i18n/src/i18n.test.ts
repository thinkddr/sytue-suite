/**
 * Tests de los cimientos de i18n: negociación de idioma (donde de verdad se
 * falla: `q` ausente, orden por `q`, subetiquetas regionales, basura), plurales
 * por `Intl.PluralRules`, interpolación y —el que evita el desastre silencioso—
 * que los cinco catálogos tienen exactamente las mismas claves con la misma
 * forma. Ejecutar: `bun test`.
 */
import { describe, expect, test } from 'bun:test';
import {
  DEFAULT_LOCALE,
  LOCALES,
  LOCALE_NAMES,
  catalogs,
  detectLocale,
  formatCurrency,
  formatNumber,
  formatTimeAgo,
  htmlLangAttrs,
  isLocale,
  negotiateLocale,
  parseAcceptLanguage,
  readLocaleCookie,
  translate,
  createAppTranslator,
  documentLocale,
  loadAppTranslator,
  type AppCatalog,
  type Locale,
} from './index.js';

describe('negociación de Accept-Language', () => {
  test('elige el primer idioma soportado respetando la región', () => {
    expect(negotiateLocale('fr-CA,fr;q=0.9,en;q=0.8')).toBe('fr');
  });

  test('`q` ausente vale 1', () => {
    expect(negotiateLocale('en;q=0.5, de')).toBe('de');
  });

  test('ordena por `q`, no por orden de aparición', () => {
    expect(negotiateLocale('es;q=0.3,it;q=0.9')).toBe('it');
  });

  test('a igual `q` gana el que aparece antes', () => {
    expect(negotiateLocale('it;q=0.8,de;q=0.8')).toBe('it');
  });

  test('un idioma que no hablamos deja paso al siguiente candidato', () => {
    expect(negotiateLocale('ja-JP,ja;q=0.9,fr;q=0.4')).toBe('fr');
  });

  test('si no hablamos ninguno, el idioma base', () => {
    expect(negotiateLocale('tr-TR,ja;q=0.9')).toBe(DEFAULT_LOCALE);
  });

  test('`q=0` es un rechazo explícito', () => {
    expect(negotiateLocale('de;q=0, es;q=0.1')).toBe('es');
  });

  test('cabecera vacía, ausente o basura ⇒ idioma base', () => {
    expect(negotiateLocale('')).toBe(DEFAULT_LOCALE);
    expect(negotiateLocale(null)).toBe(DEFAULT_LOCALE);
    expect(negotiateLocale(undefined)).toBe(DEFAULT_LOCALE);
    expect(negotiateLocale(';;;,,,')).toBe(DEFAULT_LOCALE);
    expect(negotiateLocale('de;q=abc')).toBe(DEFAULT_LOCALE);
  });

  test('el comodín `*` no elige por nosotros', () => {
    expect(negotiateLocale('*')).toBe(DEFAULT_LOCALE);
  });

  test('la etiqueta no distingue mayúsculas', () => {
    expect(parseAcceptLanguage('FR-CA')).toBe('fr');
    expect(parseAcceptLanguage('de-AT')).toBe('de');
    expect(parseAcceptLanguage('pt-BR')).toBe('pt');
    expect(parseAcceptLanguage('es-MX,es;q=0.9')).toBe('es');
    expect(parseAcceptLanguage('no')).toBe('nb');
    expect(parseAcceptLanguage('nn-NO,en;q=0.5')).toBe('nb');
  });
});

describe('cookie de preferencia', () => {
  test('manda sobre la cabecera', () => {
    expect(negotiateLocale('fr-CA,fr;q=0.9', 'theme=dark; sytue_locale=it; x=1')).toBe('it');
  });

  test('una cookie con un idioma que no hablamos no bloquea la cabecera', () => {
    expect(negotiateLocale('de', 'sytue_locale=ja')).toBe('de');
  });

  test('sin cookie, null', () => {
    expect(readLocaleCookie('theme=dark')).toBeNull();
    expect(readLocaleCookie('')).toBeNull();
    expect(readLocaleCookie(null)).toBeNull();
  });
});

describe('interpolación', () => {
  test('sustituye las variables', () => {
    expect(translate('es', 'account.menuFor', { name: 'Ada' })).toBe('Cuenta de Ada');
    expect(translate('de', 'account.organization', { tenant: 'ACME' })).toBe('Organisation: ACME');
  });

  test('los números se formatean según el locale', () => {
    expect(translate('de', 'search.resultCount', { count: 1234 })).toBe('1.234 Ergebnisse');
    expect(translate('en', 'search.resultCount', { count: 1234 })).toBe('1,234 results');
  });

  test('una variable sin valor deja el hueco visible en vez de romper', () => {
    expect(translate('en', 'search.emptyTitle')).toContain('{query}');
  });
});

describe('plurales por Intl.PluralRules', () => {
  test('inglés: singular y plural', () => {
    expect(translate('en', 'search.resultCount', { count: 1 })).toBe('1 result');
    expect(translate('en', 'search.resultCount', { count: 0 })).toBe('0 results');
  });

  test('francés: el 0 va en singular (donde `n === 1` se equivocaría)', () => {
    expect(translate('fr', 'search.resultCount', { count: 0 })).toBe('0 résultat');
    expect(translate('fr', 'search.resultCount', { count: 2 })).toBe('2 résultats');
  });

  test('italiano: la concordancia cambia con la cantidad', () => {
    expect(translate('it', 'notifications.unread', { count: 1 })).toBe('1 non letta');
    expect(translate('it', 'notifications.unread', { count: 3 })).toBe('3 non lette');
  });

  test('maltés: sin forma `two`, el 2 va en `few` (plural) y no en `other` (singular)', () => {
    expect(translate('mt', 'search.resultCount', { count: 2 })).toBe('2 riżultati');
    expect(translate('mt', 'search.resultCount', { count: 20 })).toBe('20 riżultat');
  });
});

describe('catálogos', () => {
  const keys = (locale: (typeof LOCALES)[number]): string[] => Object.keys(catalogs[locale]).sort();

  test('los cinco tienen exactamente las mismas claves', () => {
    for (const locale of LOCALES) {
      expect(keys(locale)).toEqual(keys(DEFAULT_LOCALE));
    }
  });

  test('la forma de cada mensaje coincide con la del inglés (texto o plural)', () => {
    const base = catalogs[DEFAULT_LOCALE] as Record<string, unknown>;
    for (const locale of LOCALES) {
      const catalog = catalogs[locale] as Record<string, unknown>;
      for (const key of Object.keys(base)) {
        expect(`${locale}:${key}:${typeof catalog[key]}`).toBe(
          `${locale}:${key}:${typeof base[key]}`,
        );
      }
    }
  });

  test('ninguna traducción se quedó vacía', () => {
    for (const locale of LOCALES) {
      for (const value of Object.values(catalogs[locale])) {
        const forms = typeof value === 'string' ? [value] : Object.values(value);
        for (const form of forms) expect(form.trim().length).toBeGreaterThan(0);
      }
    }
  });

  test('cada idioma se ofrece con su propio nombre', () => {
    expect(LOCALE_NAMES).toEqual({
      en: 'English',
      bg: 'Български',
      cs: 'Čeština',
      da: 'Dansk',
      de: 'Deutsch',
      el: 'Ελληνικά',
      es: 'Español',
      et: 'Eesti',
      fi: 'Suomi',
      fr: 'Français',
      ga: 'Gaeilge',
      hr: 'Hrvatski',
      hu: 'Magyar',
      is: 'Íslenska',
      it: 'Italiano',
      lt: 'Lietuvių',
      lv: 'Latviešu',
      mt: 'Malti',
      nb: 'Norsk bokmål',
      nl: 'Nederlands',
      pl: 'Polski',
      pt: 'Português',
      ro: 'Română',
      sk: 'Slovenčina',
      sl: 'Slovenščina',
      sv: 'Svenska',
    });
  });
});

describe('formato', () => {
  test('el inglés se formatea a la europea, no a la estadounidense', () => {
    // en-GB: separador de miles con coma pero moneda en euros por delante.
    expect(formatCurrency('en', 1234.5)).toContain('€');
    expect(formatNumber('en', 1234.5)).toBe('1,234.5');
    expect(formatNumber('de', 1234.5)).toBe('1.234,5');
  });

  test('la antigüedad usa la forma natural de cada idioma', () => {
    const now = Date.parse('2026-07-28T12:00:00Z');
    expect(formatTimeAgo('en', new Date(now - 30_000), now)).toBe('now');
    expect(formatTimeAgo('es', new Date(now - 30_000), now)).toBe('ahora');
    // `numeric: 'auto'` ⇒ «ayer», no «hace 1 día».
    expect(formatTimeAgo('de', new Date(now - 26 * 3_600_000), now)).toBe('gestern');
  });
});

describe('atributos del documento', () => {
  test('lang y dir en un único punto', () => {
    expect(htmlLangAttrs('fr')).toEqual({ lang: 'fr', dir: 'ltr' });
  });

  test('isLocale filtra lo que no hablamos', () => {
    expect(isLocale('es')).toBe(true);
    expect(isLocale('ja')).toBe(false);
    expect(isLocale(null)).toBe(false);
  });

  test('sin DOM, detectLocale cae al idioma base', () => {
    expect(detectLocale()).toBe(DEFAULT_LOCALE);
  });
});

// ---------------------------------------------------------------------------
// Catálogos por app
// ---------------------------------------------------------------------------

const enApp = {
  'project.create': 'Create project',
  'project.count': { one: '{count} project', other: '{count} projects' },
} as const;

// Anotado, no `satisfies`: con `as const` el catálogo inglés tiene claves `readonly` y
// valores literales, y de ahí `createAppTranslator` no podía inferir el tipo del catálogo.
const appCatalogs: Record<Locale, AppCatalog<typeof enApp>> = {
  en: enApp,
  es: {
    'project.create': 'Crear proyecto',
    'project.count': { one: '{count} proyecto', other: '{count} proyectos' },
  },
  fr: {
    'project.create': 'Créer un projet',
    'project.count': { one: '{count} projet', other: '{count} projets' },
  },
  de: {
    'project.create': 'Projekt erstellen',
    'project.count': { one: '{count} Projekt', other: '{count} Projekte' },
  },
  it: {
    'project.create': 'Crea progetto',
    'project.count': { one: '{count} progetto', other: '{count} progetti' },
  },
};

const appT = createAppTranslator(appCatalogs);

test('el catálogo de una app traduce en los cinco idiomas', () => {
  expect(appT('en')('project.create')).toBe('Create project');
  expect(appT('es')('project.create')).toBe('Crear proyecto');
  expect(appT('fr')('project.create')).toBe('Créer un projet');
  expect(appT('de')('project.create')).toBe('Projekt erstellen');
  expect(appT('it')('project.create')).toBe('Crea progetto');
});

// La `t` de un idioma tiene que ser SIEMPRE LA MISMA función.
//
// Devolvía una closure nueva en cada llamada, y como los componentes hacen
// `const t = useT(locale)` en el cuerpo del render, `t` cambiaba de identidad en
// cada render. Cualquier `useEffect(..., [..., t])` pasaba a ejecutarse siempre;
// el del compositor de Sytue Mail hace `setEditorKey(k => k + 1)`, así que era un
// bucle infinito que reseteaba los campos en cada vuelta y dejaba el compositor
// inservible: no se podía escribir ni el destinatario.
//
// Se compara por IDENTIDAD (toBe) a propósito: que traduzca igual no basta, lo
// que rompe los efectos es que la referencia cambie.
test('la t de un idioma es estable entre llamadas', () => {
  expect(appT('es')).toBe(appT('es'));
  expect(appT('en')).toBe(appT('en'));
  // Y no se comparte entre idiomas, que sería el error contrario.
  expect(appT('es')).not.toBe(appT('en'));
  // La memoización no puede haber roto lo que traduce.
  expect(appT('es')('project.create')).toBe('Crear proyecto');
});

test('los plurales de una app los decide Intl, no un if', () => {
  expect(appT('en')('project.count', { count: 1 })).toBe('1 project');
  expect(appT('en')('project.count', { count: 5 })).toBe('5 projects');
  // El francés pone el 0 en SINGULAR: es el caso que un `count === 1` se come.
  expect(appT('fr')('project.count', { count: 0 })).toBe('0 projet');
  expect(appT('es')('project.count', { count: 0 })).toBe('0 proyectos');
});

// ---------------------------------------------------------------------------
// Un solo catálogo en el navegador
// ---------------------------------------------------------------------------
//
// Los cinco catálogos de cada app viajaban enteros al navegador porque
// `createAppTranslator` los pide los cinco: medido en Sytue Admin, 895.861 B
// (246.097 B gzip) descargados, parseados y ejecutados en cada carga para usar
// una quinta parte. Un documento tiene UN idioma.

/** Finge el `<html lang>` que escribe el SSR. */
function ponerLang(v: string): void {
  (globalThis as { document?: unknown }).document = { documentElement: { lang: v } };
}

// Se devuelve lo que hubiera (happy-dom, si algún test de componente lo
// registró en este mismo proceso) en vez de borrarlo a secas.
const documentoPrevio = (globalThis as { document?: unknown }).document;

function quitarDocumento(): void {
  if (documentoPrevio === undefined) delete (globalThis as { document?: unknown }).document;
  else (globalThis as { document?: unknown }).document = documentoPrevio;
}

describe('documentLocale', () => {
  test('lee el idioma que el servidor escribió en <html lang>', () => {
    ponerLang('fr');
    expect(documentLocale()).toBe('fr');
    quitarDocumento();
  });

  // El sitio de marketing escribe `es-ES` (la forma completa es la que quieren
  // los buscadores) y las once apps escriben `es`. Leerlo tal cual hacía que www
  // se hidratara en INGLÉS con el idioma bien negociado.
  test('acepta BCP-47 con región', () => {
    for (const [lang, esperado] of [
      ['es-ES', 'es'],
      ['en-GB', 'en'],
      ['de-DE', 'de'],
      ['IT-it', 'it'],
    ] as const) {
      ponerLang(lang);
      expect(documentLocale()).toBe(esperado);
    }
    quitarDocumento();
  });

  test('cae al inglés con un idioma que no es nuestro o sin documento', () => {
    ponerLang('ja-JP');
    expect(documentLocale()).toBe(DEFAULT_LOCALE);
    ponerLang('');
    expect(documentLocale()).toBe(DEFAULT_LOCALE);
    quitarDocumento();
    expect(documentLocale()).toBe(DEFAULT_LOCALE);
  });
});

describe('loadAppTranslator', () => {
  const deMentira = {
    en: { saludo: 'Hello' },
    es: { saludo: 'Hola' },
    fr: { saludo: 'Bonjour' },
    de: { saludo: 'Hallo' },
    it: { saludo: 'Ciao' },
  };

  /** Imita `import.meta.glob('./messages/*.ts')` y apunta qué chunk se pidió. */
  function cargadores(pedidos: string[]): Record<string, () => Promise<unknown>> {
    return Object.fromEntries(
      LOCALES.map((l) => [
        `./messages/${l}.ts`,
        () => {
          pedidos.push(l);
          return Promise.resolve({ [l]: deMentira[l] });
        },
      ]),
    );
  }

  test('baja UN catálogo: el del documento, y ningún otro', async () => {
    ponerLang('de-DE');
    const pedidos: string[] = [];
    const useT = await loadAppTranslator<{ saludo: string }>(cargadores(pedidos));
    expect(pedidos).toEqual(['de']);
    expect(useT('de')('saludo')).toBe('Hallo');
    quitarDocumento();
  });

  test('el locale que pida la isla no puede traer otro idioma', async () => {
    ponerLang('es');
    const pedidos: string[] = [];
    const useT = await loadAppTranslator<{ saludo: string }>(cargadores(pedidos));
    // Da igual lo que pida: el documento ya está pintado en español.
    expect(useT('fr')('saludo')).toBe('Hola');
    expect(useT('en')('saludo')).toBe('Hola');
    expect(pedidos).toEqual(['es']);
    quitarDocumento();
  });

  test('la t es la MISMA función entre llamadas (efectos con [t] en las islas)', async () => {
    ponerLang('it');
    const useT = await loadAppTranslator<{ saludo: string }>(cargadores([]));
    expect(useT('it')).toBe(useT('it'));
    quitarDocumento();
  });

  // Sin catálogo se devuelve la clave: una interfaz sin traducir es un defecto,
  // una pantalla en blanco es una caída.
  test('si el chunk no llega, devuelve la clave en vez de reventar', async () => {
    ponerLang('es');
    const useT = await loadAppTranslator<{ saludo: string }>({});
    expect(useT('es')('saludo')).toBe('saludo');
    quitarDocumento();
  });

  /** Cargador que RECHAZA las primeras `fallos` veces, como el de Vite cuando el
   *  chunk o su `modulepreload` no bajan. */
  function cargadorQueFalla(fallos: number, intentos: { n: number }) {
    return {
      './messages/es.ts': () => {
        intentos.n += 1;
        return intentos.n <= fallos
          ? Promise.reject(new Error('Unable to preload module'))
          : Promise.resolve({ es: deMentira.es });
      },
    };
  }

  // El await es de nivel superior: una promesa rechazada deja el módulo sin
  // evaluar y quien importe `useT` lo ve `undefined`. Es lo que dejó Sytue Admin
  // en blanco en Safari.
  test('un chunk que RECHAZA no tumba el módulo: devuelve la clave', async () => {
    ponerLang('es');
    const intentos = { n: 0 };
    const useT = await loadAppTranslator<{ saludo: string }>(cargadorQueFalla(9, intentos));
    expect(useT('es')('saludo')).toBe('saludo');
    expect(intentos.n).toBe(2); // el original y un reintento, no más
    quitarDocumento();
  });

  test('un fallo pasajero se reintenta y sí traduce', async () => {
    ponerLang('es');
    const intentos = { n: 0 };
    const useT = await loadAppTranslator<{ saludo: string }>(cargadorQueFalla(1, intentos));
    expect(useT('es')('saludo')).toBe('Hola');
    expect(intentos.n).toBe(2);
    quitarDocumento();
  });
});
