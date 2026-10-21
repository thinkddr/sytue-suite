/**
 * Formato de fechas, números, moneda y listas por locale. Todo es `Intl` de la
 * plataforma: ni una tabla de meses a mano, ni una dependencia. Aquí vive el
 * único mapa locale → etiqueta BCP-47 (`intlTag`), que es lo que hace que el
 * inglés salga en formato europeo y no estadounidense.
 */
import { intlTag, type Locale } from './locale.js';

export type DateLike = Date | string | number;

function toDate(value: DateLike): Date {
  return value instanceof Date ? value : new Date(value);
}

/** Fecha (por defecto media: «28 Jul 2026», «28 jul 2026», «28.07.2026»). */
export function formatDate(
  locale: Locale,
  value: DateLike,
  options: Intl.DateTimeFormatOptions = { dateStyle: 'medium' },
): string {
  return toDate(value).toLocaleDateString(intlTag(locale), options);
}

/** Hora (por defecto corta; el reloj de 12/24 h lo decide el locale). */
export function formatTime(
  locale: Locale,
  value: DateLike,
  options: Intl.DateTimeFormatOptions = { timeStyle: 'short' },
): string {
  return toDate(value).toLocaleTimeString(intlTag(locale), options);
}

export function formatDateTime(
  locale: Locale,
  value: DateLike,
  options: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' },
): string {
  return toDate(value).toLocaleString(intlTag(locale), options);
}

export function formatNumber(
  locale: Locale,
  value: number,
  options?: Intl.NumberFormatOptions,
): string {
  return value.toLocaleString(intlTag(locale), options);
}

/**
 * Moneda. EUR por defecto: la suite factura en euros. La POSICIÓN del símbolo y
 * el separador decimal los pone el locale (1.234,50 € en alemán, €1,234.50 en
 * inglés europeo), que es justo lo que un formateador fijo hace mal.
 */
export function formatCurrency(
  locale: Locale,
  amount: number,
  currency = 'EUR',
  options?: Intl.NumberFormatOptions,
): string {
  return amount.toLocaleString(intlTag(locale), { style: 'currency', currency, ...options });
}

/** Porcentaje. `value` es la fracción (0.42 ⇒ «42 %»). */
export function formatPercent(
  locale: Locale,
  value: number,
  options?: Intl.NumberFormatOptions,
): string {
  return value.toLocaleString(intlTag(locale), {
    style: 'percent',
    maximumFractionDigits: 1,
    ...options,
  });
}

/** Enumeración natural: «a, b and c» / «a, b y c» / «a, b und c». */
export function formatList(
  locale: Locale,
  items: readonly string[],
  type: 'conjunction' | 'disjunction' = 'conjunction',
): string {
  return new Intl.ListFormat(intlTag(locale), { type, style: 'long' }).format(items);
}

export function formatRelativeTime(
  locale: Locale,
  value: number,
  unit: Intl.RelativeTimeFormatUnit,
): string {
  return new Intl.RelativeTimeFormat(intlTag(locale), {
    numeric: 'auto',
    style: 'short',
  }).format(value, unit);
}

/**
 * Antigüedad legible de un instante: «now», «5 min ago», «yesterday»… y, a
 * partir de una semana, fecha corta (día y mes), que es más útil que «hace 43
 * días». `numeric: 'auto'` es lo que da «ayer/hier/gestern» en vez de «hace 1
 * día», y el caso «ahora» sale de `format(0, 'second')`: no hace falta una clave
 * de catálogo para una palabra que CLDR ya tiene en los cinco idiomas.
 */
export function formatTimeAgo(locale: Locale, value: DateLike, now: number = Date.now()): string {
  const date = toDate(value);
  const seconds = Math.round((now - date.getTime()) / 1_000);
  if (seconds < 60) return formatRelativeTime(locale, 0, 'second');
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return formatRelativeTime(locale, -minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (hours < 24) return formatRelativeTime(locale, -hours, 'hour');
  const days = Math.round(hours / 24);
  if (days < 7) return formatRelativeTime(locale, -days, 'day');
  return formatDate(locale, date, { day: 'numeric', month: 'short' });
}
