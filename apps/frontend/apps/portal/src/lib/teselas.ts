import { segmentoDeFicha, suiteApex } from '@sytue/config';
import { DEFAULT_LOCALE, type Locale } from '@sytue/i18n';

/**
 * Adónde lleva el «Más información» de una tesela desactivada: la ficha pública del
 * producto en el sitio de la suite, que dice «Solo para organizaciones» antes del
 * titular y explica qué es. Es la ayuda que existe hoy; no hay centro de ayuda aparte.
 *
 * En el idioma de quien mira: el sitio sirve el idioma base sin prefijo y los demás
 * bajo `/<idioma>` (`localePath` de www). Va el SEGMENTO público de la ficha
 * (`segmentoDeFicha`, la misma tabla que usa www), no la clave: Sytue Web es `pages`
 * por dentro y su ficha está en `/product/web`; `/product/pages` solo redirige con una
 * página intermedia.
 */
export function ayudaSoloOrganizaciones(key: string, locale: Locale): string {
  const idioma = locale === DEFAULT_LOCALE ? '' : `/${locale}`;
  return `https://www.${suiteApex}${idioma}/product/${encodeURIComponent(segmentoDeFicha(key))}`;
}
