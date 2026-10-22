/**
 * Traducción: catálogo + variables + plurales. La categoría de plural la decide
 * `Intl.PluralRules` y no un `count === 1`, porque «uno frente a muchos» no es
 * universal (en francés el 0 va en singular, y en cuanto entre una lengua con
 * `few`/`many` el `if` se rompe en silencio). La API de la plataforma ya lo sabe.
 */
import { DEFAULT_LOCALE, intlTag, isLocale, type Locale } from './locale.js';
import { formatNumber } from './format.js';
import { catalogs, type MessageKey, type PluralForms } from './messages/index.js';

/** Variables de interpolación. Los números se formatean según el locale. */
export type Vars = Record<string, string | number>;

/** Función de traducción ya atada a un locale. */
export type Translate = (key: MessageKey, vars?: Vars) => string;

const PLACEHOLDER = /\{(\w+)\}/g;

// `Intl.PluralRules` se construye una vez por idioma: es el objeto Intl que más
// se llama (cada `t` con cantidad) y crearlo no es gratis.
const pluralRules = new Map<Locale, Intl.PluralRules>();

function rulesFor(locale: Locale): Intl.PluralRules {
  let rules = pluralRules.get(locale);
  if (rules === undefined) {
    rules = new Intl.PluralRules(intlTag(locale));
    pluralRules.set(locale, rules);
  }
  return rules;
}

function selectPlural(locale: Locale, forms: PluralForms, vars: Vars | undefined): string {
  const raw = Number(vars?.['count'] ?? 0);
  const count = Number.isFinite(raw) ? raw : 0;
  const category = rulesFor(locale).select(count);
  // CLDR 42 separó `two` de `few` en maltés: sin `two`, el 2 usa `few` como antes, no el `other` en singular.
  return forms[category] ?? (category === 'two' ? forms.few : undefined) ?? forms.other;
}

function interpolate(locale: Locale, template: string, vars: Vars | undefined): string {
  if (vars === undefined) return template;
  return template.replace(PLACEHOLDER, (match: string, name: string) => {
    const value = vars[name];
    // Hueco sin valor: se deja tal cual. Es feo y por eso se ve en una revisión;
    // reventar en tiempo de ejecución por una etiqueta sería mucho peor.
    if (value === undefined) return match;
    return typeof value === 'number' ? formatNumber(locale, value) : value;
  });
}

/** Traduce una clave. `vars.count` es la cantidad que elige la forma plural. */
export function translate(locale: Locale, key: MessageKey, vars?: Vars): string {
  const entry = catalogs[locale][key];
  const template = typeof entry === 'string' ? entry : selectPlural(locale, entry, vars);
  return interpolate(locale, template, vars);
}

/** `t` atada a un locale, que es como la usan los componentes. */
export function createTranslate(locale: Locale): Translate {
  return (key, vars) => translate(locale, key, vars);
}

// ---------------------------------------------------------------------------
// Catálogos por app
// ---------------------------------------------------------------------------
//
// El catálogo de arriba es el del CROMO compartido: lo que se ve en las once apps.
// Los textos de cada app viven en la app, no aquí, por dos razones:
//
//   1. Un catálogo único con las once apps dentro sería un fichero que todo el
//      mundo edita a la vez — el peor sitio posible para un monorepo con varias
//      manos trabajando en paralelo.
//   2. Ninguna app carga los textos de las otras diez.
//
// Lo que sí es común es la MECÁNICA, y por eso se comparte desde aquí: la misma
// interpolación, los mismos plurales por `Intl.PluralRules` y, sobre todo, la misma
// garantía de que el inglés manda y las otras cuatro se comprueban en compilación.

/** Una entrada de catálogo: texto llano o formas de plural. */
export type MessageEntry = string | PluralForms;

/**
 * Catálogo de una app derivado del INGLÉS: cada clave conserva su forma (texto o
 * plural). Es lo que hace que una clave nueva —o convertida en plural— en inglés
 * rompa la compilación de los demás idiomas en vez de aparecer sin traducir en
 * producción.
 */
export type AppCatalog<EN extends Record<string, MessageEntry>> = {
  [K in keyof EN]: EN[K] extends string ? string : PluralForms;
};

/**
 * Los catálogos de una app: el inglés siempre, el resto a medida que llegan. Los
 * idiomas se publican app a app, así que una app puede ir por detrás de `LOCALES`;
 * el idioma que aún no tiene se sirve en inglés, que es lo que ya hacía la red de
 * `createAppTranslator` en tiempo de ejecución.
 */
export type AppCatalogs<EN extends Record<string, MessageEntry>> = {
  en: AppCatalog<EN>;
} & Partial<Record<Locale, AppCatalog<EN>>>;

/**
 * Crea la `t` de una app a partir de sus catálogos.
 *
 * Se le pasa el juego que tenga la app y devuelve la función atada a un locale,
 * para que el componente no tenga que saber de catálogos:
 *
 *	const useAppT = createAppTranslator({ en, es, fr, de, it })
 *	const t = useAppT(locale)
 *	t('project.create')            // texto
 *	t('project.count', { count })  // plural resuelto por Intl
 */
export function createAppTranslator<EN extends Record<string, MessageEntry>>(
  appCatalogs: AppCatalogs<EN>,
): (locale: Locale) => (key: keyof EN & string, vars?: Vars) => string {
  // UNA `t` POR IDIOMA, memoizada. Devolvía una closure NUEVA en cada llamada, y
  // como los componentes hacen `const t = useT(locale)` en el cuerpo del render,
  // `t` cambiaba de identidad en cada render.
  //
  // Eso convierte cualquier `useEffect(..., [..., t])` en «ejecutar en cada
  // render». Hay diecisiete en la suite. La mayoría solo desperdiciaban trabajo,
  // pero el del compositor de Sytue Mail hace `setEditorKey(k => k + 1)`: efecto →
  // cambio de estado → render → `t` nueva → efecto, sin parar. Con el compositor
  // abierto era un bucle infinito que reseteaba «Para», «Asunto» y el cuerpo en
  // cada vuelta, así que no se podía escribir NADA.
  //
  // La memoización es segura y no guarda estado de petición: los catálogos son
  // estáticos y la traducción es pura (mismo idioma y misma clave ⇒ mismo texto),
  // así que dos llamadas con el mismo idioma pueden compartir la misma función.
  const porIdioma = new Map<Locale, (key: keyof EN & string, vars?: Vars) => string>();
  return (locale) => {
    const memo = porIdioma.get(locale);
    if (memo) return memo;
    const t = (key: keyof EN & string, vars?: Vars): string => {
      // Doble red, y no por gusto: un catálogo de app que no cubra el idioma
      // activo tiraba la PÁGINA ENTERA con «undefined is not an object» desde el
      // render del servidor —así se cayó Sytue DevX— y una clave que aún no se
      // ha traducido hacía lo mismo. Una interfaz a medio traducir es un defecto;
      // una pantalla en blanco es una caída.
      const catalog = appCatalogs[locale] ?? appCatalogs[DEFAULT_LOCALE];
      const entry = catalog?.[key] ?? appCatalogs[DEFAULT_LOCALE]?.[key];
      if (entry === undefined) return key;
      const template = typeof entry === 'string' ? entry : selectPlural(locale, entry, vars);
      return interpolate(locale, template, vars);
    };
    porIdioma.set(locale, t);
    return t;
  };
}

/**
 * Idioma del documento. Lo escribió el servidor en `<html lang>` al renderizar,
 * así que ya está en el DOM cuando se hidrata la primera isla: el navegador no
 * tiene que negociarlo otra vez ni esperar a ninguna petición para saberlo.
 *
 * Se queda con la subetiqueta primaria porque las once apps escriben `es` pero
 * el sitio de marketing escribe `es-ES` —la forma completa es la que quieren los
 * buscadores—, y leerlo tal cual hacía que www cayera al inglés con el idioma
 * bien negociado. Es BCP-47: lo que va antes del primer guion es el idioma.
 */
export function documentLocale(): Locale {
  const lang = typeof document === 'undefined' ? '' : document.documentElement.lang;
  const primaria = lang.split('-')[0]?.toLowerCase() ?? '';
  return isLocale(primaria) ? primaria : DEFAULT_LOCALE;
}

/**
 * La `useT` del NAVEGADOR, con un solo catálogo: el del idioma que se sirve.
 *
 * `createAppTranslator` recibe todos los catálogos, y pedirlos obliga a
 * importarlos, así que todos acababan en el bundle del cliente. Medido en
 * Sytue Admin con cinco: 895.861 B (246.097 B gzip) de JS descargado, parseado y
 * ejecutado en cada carga para usar una quinta parte. Un documento tiene UN idioma.
 *
 * Se le pasa el mapa de `import.meta.glob('./messages/*.ts')` —sin `eager`, para
 * que Vite deje un chunk por idioma— y solo se baja el que toca. El `locale` que
 * pida la isla se ignora a propósito: el documento ya está pintado en un idioma,
 * y pedir otro distinto solo puede ser un error. Devolver SIEMPRE la misma `t`
 * mantiene además su identidad estable entre renders, que es lo que evita el
 * bucle de efectos que documenta `createAppTranslator`.
 */
export async function loadAppTranslator<EN extends Record<string, MessageEntry>>(
  cargadores: Record<string, () => Promise<unknown>>,
): Promise<(locale: Locale) => (key: keyof EN & string, vars?: Vars) => string> {
  const locale = documentLocale();
  // Idioma que la app aún no tiene: se baja el inglés, que es lo que pintó el
  // servidor. Sin esto no había cargador, y la isla hidrataba con las claves en
  // crudo encima de una página bien pintada.
  const servido = `./messages/${locale}.ts` in cargadores ? locale : DEFAULT_LOCALE;
  const cargar = cargadores[`./messages/${servido}.ts`];
  // El cargador de Vite RECHAZA si el chunk o su `modulepreload` no llegan, y este
  // `await` es de nivel superior: sin `catch`, el módulo entero se queda sin evaluar
  // y quien importe `useT` lo ve `undefined`. Eso es lo que dejó Sytue Admin en
  // blanco en Safari («b is not a function», la isla no hidrata) por un solo chunk
  // que no bajó. Un reintento cubre el fallo de red pasajero; si tampoco llega, se
  // sigue sin catálogo y `createAppTranslator` devuelve la clave: una interfaz sin
  // traducir es un defecto, una pantalla en blanco es una caída.
  const modulo = (await cargar?.().catch(() => cargar().catch(() => undefined))) as
    Record<string, AppCatalog<EN>> | undefined;
  const t = createAppTranslator<EN>({ [locale]: modulo?.[servido] } as AppCatalogs<EN>)(locale);
  return () => t;
}
