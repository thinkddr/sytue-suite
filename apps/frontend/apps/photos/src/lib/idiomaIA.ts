/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * En qué idioma se le pide a Sytue AI que escriba lo que la persona va a leer.
 *
 * Lo comparten describir (las etiquetas nombran luego un álbum) y organizar en
 * álbumes (los títulos son los de «Álbumes»). Es el `idiomasIA` de
 * `apps/nucleo/api/photos_ia.go`.
 */

/**
 * Nombre en inglés de cada idioma de producto, porque las instrucciones están en
 * inglés. Lo que no está aquí cae al inglés base, como el resto de la interfaz:
 * así en la instrucción nunca entra texto que no esté escrito en este fichero.
 */
export const IDIOMAS_IA: Readonly<Record<string, string>> = {
  en: 'English',
  de: 'German',
  es: 'Spanish',
  fr: 'French',
  it: 'Italian',
};

/**
 * El idioma de la sesión con su nombre en inglés, o el inglés si no es de producto.
 * `Object.hasOwn` y no `IDIOMAS_IA[locale] ??` a secas: con «constructor» el objeto
 * devolvía la función de su prototipo, y acababa escrita dentro de la instrucción.
 */
export const idiomaParaIA = (locale: string): string =>
  Object.hasOwn(IDIOMAS_IA, locale) ? IDIOMAS_IA[locale]! : 'English';
