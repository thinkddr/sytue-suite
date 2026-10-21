/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Rutas por idioma. Vive APARTE del catálogo (`./index.ts`) por una razón muy
 * concreta de peso: las islas Preact necesitan construir enlaces, pero no los
 * textos —esos se los da el servidor ya resueltos—, y un solo import desde el
 * módulo del catálogo arrastraría los 26 idiomas completos (con las
 * páginas legales dentro) al bundle del navegador. Aquí no hay más que cadenas.
 */
import { DEFAULT_LOCALE, LOCALES, isLocale, type Locale } from '@sytue/i18n';

/**
 * Ruta de una página en un idioma. `path` es SIEMPRE la ruta base (la del
 * inglés, sin prefijo): '/', '/downloads', '/product/mail'.
 */
export function localePath(locale: Locale, path = '/'): string {
  const clean = path === '/' ? '' : `/${path.replace(/^\/+|\/+$/g, '')}`;
  if (locale === DEFAULT_LOCALE) return clean === '' ? '/' : clean;
  return `/${locale}${clean}`;
}

/**
 * `getStaticPaths` de las rutas `[...locale]/…`: una variante por idioma. El
 * inglés se genera con el parámetro a `undefined`, que es como Astro produce la
 * ruta sin prefijo (`/downloads` en vez de `/en/downloads`).
 */
export function localeParams(): { params: { locale: string | undefined } }[] {
  return LOCALES.map((locale) => ({
    params: { locale: locale === DEFAULT_LOCALE ? undefined : locale },
  }));
}

/** Locale de una ruta ya generada. Sin parámetro = raíz = idioma base. */
export function localeFromParam(param?: string): Locale {
  return isLocale(param) ? param : DEFAULT_LOCALE;
}
