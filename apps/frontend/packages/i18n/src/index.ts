/**
 * @sytue/i18n — los cimientos multiidioma de la plataforma. Cinco idiomas de
 * producto (inglés BASE + español, francés, alemán e italiano) según
 * docs/CONVENTIONS.md §13.1: una función sin sus cinco idiomas no está acabada.
 *
 * Sin dependencias: todo lo que hace falta (plurales, fechas, números, moneda,
 * listas) lo trae `Intl`. El binding para Preact vive en `@sytue/i18n/preact`
 * para que el core siga sirviendo en SSR, en el BFF y en scripts.
 */

export type { Locale } from './locale.js';
export {
  LOCALES,
  DEFAULT_LOCALE,
  LOCALE_NAMES,
  LOCALE_COOKIE,
  intlTag,
  isLocale,
  localeDir,
  htmlLangAttrs,
  readLocaleCookie,
  parseAcceptLanguage,
  negotiateLocale,
  detectLocale,
  persistLocale,
  applyDocumentLocale,
} from './locale.js';

export type { Catalog, MessageKey, PluralForms } from './messages/index.js';
export { catalogs } from './messages/index.js';

export type { Vars, Translate } from './translate.js';
export {
  translate,
  createTranslate,
  createAppTranslator,
  loadAppTranslator,
  documentLocale,
} from './translate.js';
export type { AppCatalog, AppCatalogs, MessageEntry } from './translate.js';

export type { DateLike } from './format.js';
export {
  formatDate,
  formatTime,
  formatDateTime,
  formatNumber,
  formatCurrency,
  formatPercent,
  formatList,
  formatRelativeTime,
  formatTimeAgo,
} from './format.js';

export { plegar, contiene, rangos, terminos, rangosDeConsulta } from './plegar.js';
