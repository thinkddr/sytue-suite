/**
 * Locales de producto y su negociación. Los idiomas de docs/CONVENTIONS.md
 * §13.1 (inglés BASE + es/fr/de/it) viven aquí, junto con la única forma de
 * decidir en qué idioma se sirve una petición: cookie explícita del usuario >
 * cabecera `Accept-Language` > inglés.
 *
 * Todo lo de este módulo es puro y sin DOM salvo los tres helpers de cliente del
 * final, para que el mismo código valga en el middleware de Astro (SSR), en el
 * BFF y en el navegador.
 */

export type Locale =
  | 'en'
  | 'bg'
  | 'cs'
  | 'da'
  | 'de'
  | 'el'
  | 'es'
  | 'et'
  | 'fi'
  | 'fr'
  | 'ga'
  | 'hr'
  | 'hu'
  | 'is'
  | 'it'
  | 'lt'
  | 'lv'
  | 'mt'
  | 'nb'
  | 'nl'
  | 'pl'
  | 'pt'
  | 'ro'
  | 'sk'
  | 'sl'
  | 'sv';

/** Orden canónico de presentación: el base primero, el resto alfabético por código. */
export const LOCALES: readonly Locale[] = [
  'en',
  'bg',
  'cs',
  'da',
  'de',
  'el',
  'es',
  'et',
  'fi',
  'fr',
  'ga',
  'hr',
  'hu',
  'is',
  'it',
  'lt',
  'lv',
  'mt',
  'nb',
  'nl',
  'pl',
  'pt',
  'ro',
  'sk',
  'sl',
  'sv',
];

/** Inglés: las cadenas nacen aquí y el resto son traducciones (§13.1). */
export const DEFAULT_LOCALE: Locale = 'en';

/**
 * Nombre de cada idioma EN SU PROPIO IDIOMA. Nunca se traduce: es lo que permite
 * a alguien que no entiende la interfaz actual encontrar la suya.
 */
export const LOCALE_NAMES: Record<Locale, string> = {
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
};

/**
 * Etiqueta BCP-47 que se le pasa a `Intl`. El inglés se formatea como en-GB, NO
 * como en-US: somos una suite europea y `en` por defecto daría 12/31/2026 y $.
 * El resto de idiomas no necesitan región porque su forma por defecto ya es la
 * europea.
 */
const INTL_TAG: Record<Locale, string> = {
  en: 'en-GB',
  bg: 'bg-BG',
  cs: 'cs-CZ',
  da: 'da-DK',
  de: 'de-DE',
  el: 'el-GR',
  es: 'es-ES',
  et: 'et-EE',
  fi: 'fi-FI',
  fr: 'fr-FR',
  ga: 'ga-IE',
  hr: 'hr-HR',
  hu: 'hu-HU',
  is: 'is-IS',
  it: 'it-IT',
  lt: 'lt-LT',
  lv: 'lv-LV',
  mt: 'mt-MT',
  nb: 'nb-NO',
  nl: 'nl-NL',
  pl: 'pl-PL',
  pt: 'pt-PT',
  ro: 'ro-RO',
  sk: 'sk-SK',
  sl: 'sl-SI',
  sv: 'sv-SE',
};

export function intlTag(locale: Locale): string {
  return INTL_TAG[locale];
}

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (LOCALES as readonly string[]).includes(value);
}

/**
 * Dirección del texto. Hoy todos los idiomas son LTR; este es el punto ÚNICO que
 * habría que tocar si entra árabe o hebreo (y con él `dir` en el `<html>`).
 */
const RTL_LOCALES: readonly string[] = [];

export function localeDir(locale: Locale): 'ltr' | 'rtl' {
  return RTL_LOCALES.includes(locale) ? 'rtl' : 'ltr';
}

/** Atributos del `<html>`. Se escupen tal cual en el layout de cada app. */
export function htmlLangAttrs(locale: Locale): { lang: Locale; dir: 'ltr' | 'rtl' } {
  return { lang: locale, dir: localeDir(locale) };
}

/** Cookie de preferencia explícita. Manda sobre la cabecera del navegador. */
export const LOCALE_COOKIE = 'sytue_locale';

/** Un año: la preferencia de idioma no caduca en una sesión. */
const LOCALE_COOKIE_MAX_AGE = 60 * 60 * 24 * 365;

/**
 * Extrae el locale de una cabecera `Cookie:` cruda (o de `document.cookie`, que
 * tiene el mismo formato). Devuelve null si no está o si no es un idioma nuestro.
 */
export function readLocaleCookie(cookieHeader?: string | null): Locale | null {
  if (!cookieHeader) return null;
  for (const part of cookieHeader.split(';')) {
    const eq = part.indexOf('=');
    if (eq === -1) continue;
    if (part.slice(0, eq).trim() !== LOCALE_COOKIE) continue;
    const raw = part.slice(eq + 1).trim();
    let value = raw;
    try {
      value = decodeURIComponent(raw);
    } catch {
      // Cookie manipulada con %-escapes rotos: se ignora el valor.
      return null;
    }
    return isLocale(value) ? value : null;
  }
  return null;
}

/**
 * Mejor idioma según `Accept-Language`, o null si el navegador no pide ninguno
 * que hablemos. Ordena por `q` (ausente = 1), descarta `q=0` (rechazo explícito)
 * y compara solo la subetiqueta primaria: `fr-CA` cuenta como `fr`, y `pt-BR`,
 * que no está en la lista, deja paso al siguiente candidato.
 */
export function parseAcceptLanguage(header?: string | null): Locale | null {
  if (!header) return null;
  const candidates = header
    .split(',')
    .map((part, index) => {
      const [tag = '', ...params] = part.trim().split(';');
      // Se busca el parámetro `q` por nombre: otros parámetros no alteran el peso,
      // y un `q` presente pero ilegible (`q=abc`) NO puede colar el idioma como
      // preferido — cuenta como 0, es decir, se descarta.
      const q = params
        .map((p) => {
          const param = p.trim();
          const equals = param.indexOf('=');
          return equals >= 0 && param.slice(0, equals).trim().toLowerCase() === 'q'
            ? param.slice(equals + 1).trim()
            : null;
        })
        .find((value) => value !== null);
      const quality = q === undefined ? 1 : Number(q);
      const base = (tag.trim().toLowerCase().split('-')[0] ?? '').trim();
      // Ordenar por q y, a igualdad, por orden de aparición (el sort ya es
      // estable, pero el desempate explícito documenta la intención).
      return { base, quality: Number.isFinite(quality) ? quality : 0, index };
    })
    .filter((c) => c.quality > 0 && c.base.length > 0)
    .sort((a, b) => b.quality - a.quality || a.index - b.index);

  for (const c of candidates) {
    const base = MACROLANGUAGE[c.base] ?? c.base;
    if (isLocale(base)) return base;
  }
  return null;
}

/**
 * Etiquetas que un navegador manda para un idioma que publicamos con otro
 * código: el noruego llega como `no` (macrolengua) o `nn` (nynorsk) tanto como
 * `nb`, y solo tenemos bokmål. Sin esto, un navegador en `no` caía al inglés.
 */
const MACROLANGUAGE: Record<string, Locale> = { no: 'nb', nn: 'nb' };

/**
 * La decisión completa, en una función pura y testeable: cookie > cabecera >
 * inglés. Es la que llama el SSR con `request.headers`.
 */
export function negotiateLocale(
  acceptLanguage?: string | null,
  cookieHeader?: string | null,
): Locale {
  return readLocaleCookie(cookieHeader) ?? parseAcceptLanguage(acceptLanguage) ?? DEFAULT_LOCALE;
}

// ---- Cliente (navegador) ----

/**
 * Idioma de una isla que no recibe el suyo del servidor. Mismo orden que
 * `negotiateLocale` —la elección explícita del usuario por delante de todo— y
 * luego el `<html lang>` que pintó la página y las preferencias del navegador.
 *
 * La cookie va ANTES que `<html lang>` a propósito: hoy los layouts fijan
 * `lang="es"` a mano, y si mandara el atributo, elegir idioma en el selector no
 * haría nada. Cuando una app negocie de verdad, le pasará el locale a la shell y
 * esta función ni se llama.
 */
export function detectLocale(): Locale {
  if (typeof document === 'undefined') return DEFAULT_LOCALE;
  const fromHtml = document.documentElement.lang;
  return (
    readLocaleCookie(document.cookie) ??
    (isLocale(fromHtml) ? fromHtml : null) ??
    parseAcceptLanguage(navigator.languages?.join(',') ?? navigator.language) ??
    DEFAULT_LOCALE
  );
}

/**
 * Guarda la preferencia como cookie host-only. Las publicaciones Cloud viven
 * bajo el mismo apex, por lo que una cookie de dominio filtraría la preferencia
 * y permitiría que un sitio de usuario la sobrescribiera. En una app nueva se
 * negocia de nuevo desde `Accept-Language`; la elección posterior queda local.
 * null retira sólo esta cookie para volver a la negociación, con el mismo path.
 */
export function persistLocale(locale: Locale | null): void {
  if (typeof document === 'undefined') return;
  const secure = location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${LOCALE_COOKIE}=${locale ?? ''}; path=/; max-age=${locale === null ? 0 : LOCALE_COOKIE_MAX_AGE}; SameSite=Lax${secure}`;
}

/** Refleja el idioma en el `<html>` (lang para lectores/hyphenation, dir para RTL). */
export function applyDocumentLocale(locale: Locale): void {
  if (typeof document === 'undefined') return;
  document.documentElement.lang = locale;
  document.documentElement.dir = localeDir(locale);
}
