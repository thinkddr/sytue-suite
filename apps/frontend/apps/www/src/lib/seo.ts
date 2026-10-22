/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * seo — utilidades de SEO del sitio público de Sytue One: origen canónico, URLs
 * absolutas, alternos hreflang y datos estructurados JSON-LD (schema.org:
 * Organization, WebSite, SoftwareApplication). docs/CONVENTIONS.md §0.8,
 * docs/operations.md
 *
 * El mapa de idiomas NO vive aquí. Los 26 locales de producto y su etiqueta
 * BCP-47 los fija @sytue/i18n (`LOCALES`, `intlTag`), que es lo que usan también
 * las otras diez apps; duplicarlo aquí solo garantizaba que un día divergieran.
 * El inglés se etiqueta `en-GB` a propósito: somos una suite europea y `en` a
 * secas arrastra el formato estadounidense (12/31/2026, $).
 */
import { LOCALES, DEFAULT_LOCALE, intlTag, type Locale } from '@sytue/i18n';
import { localePath, type T } from '../i18n';

/** Origen canónico de producción (contrato §1: sytue.eu / www.sytue.eu). */
export const SITE_ORIGIN = 'https://sytue.eu';

/** Imagen social por defecto (OpenGraph/Twitter). PNG 1200x630 en /public. */
export const DEFAULT_OG_IMAGE = '/og/sytue-one.png';

/** URL absoluta canónica a partir de una ruta (sin barra final salvo raíz). */
export function absoluteUrl(path: string): string {
  if (/^https?:\/\//.test(path)) return path;
  const p = path.startsWith('/') ? path : `/${path}`;
  const normalized = p !== '/' ? p.replace(/\/$/, '') : '/';
  return `${SITE_ORIGIN}${normalized}`;
}

/** Enlace alterno hreflang. */
export interface AlternateLink {
  hreflang: string;
  href: string;
}

/**
 * Alternos hreflang de una página, a partir de su ruta BASE (la del inglés, sin
 * prefijo: '/', '/downloads', '/product/mail').
 *
 * `locales` es la lista de idiomas en los que esa página EXISTE de verdad, y es
 * obligatoria por diseño: un `hreflang` hacia una traducción que no existe manda
 * al rastreador a un 404 y le enseña a desconfiar del resto. Hoy las cinco rutas
 * existen en los 26 idiomas y todas las páginas pasan `LOCALES`, pero la
 * función no PUEDE afirmarlo por su cuenta.
 *
 * `x-default` apunta al inglés, y solo si el inglés está entre los alternos.
 */
export function hreflangAlternates(path: string, locales: readonly Locale[]): AlternateLink[] {
  const alternates: AlternateLink[] = locales.map((locale) => ({
    hreflang: intlTag(locale),
    href: absoluteUrl(localePath(locale, path)),
  }));
  if (locales.includes(DEFAULT_LOCALE)) {
    alternates.push({
      hreflang: 'x-default',
      href: absoluteUrl(localePath(DEFAULT_LOCALE, path)),
    });
  }
  return alternates;
}

/** Idiomas del sitio en BCP-47, para `inLanguage` / `knowsLanguage` de JSON-LD. */
const SITE_LANGUAGES: string[] = LOCALES.map(intlTag);

/**
 * Datos estructurados JSON-LD de la organización (schema.org/Organization).
 * Se emite una sola vez (home). Marca, logo, redes y contacto europeo.
 */
export function organizationJsonLd(t: T): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    // @id estable entre idiomas: es LA MISMA entidad, no una por traducción.
    '@id': `${SITE_ORIGIN}/#organization`,
    name: 'Sytue',
    legalName: 'Sytue',
    url: SITE_ORIGIN,
    logo: absoluteUrl('/og/logo.png'),
    description: t('seo.org.description'),
    slogan: t('seo.org.slogan'),
    areaServed: 'EU',
    knowsLanguage: SITE_LANGUAGES,
    sameAs: ['https://github.com/sytue'],
  };
}

/**
 * Datos estructurados del sitio (schema.org/WebSite): habilita el sitelinks
 * searchbox y ancla la marca del dominio.
 */
export function webSiteJsonLd(): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'WebSite',
    '@id': `${SITE_ORIGIN}/#website`,
    url: SITE_ORIGIN,
    name: 'Sytue One',
    inLanguage: SITE_LANGUAGES,
    publisher: { '@id': `${SITE_ORIGIN}/#organization` },
  };
}

/**
 * Datos estructurados del producto (schema.org/SoftwareApplication): Sytue One
 * como aplicación de productividad empresarial, alojada en la UE.
 */
/**
 * El precio de la suite, en UN SOLO SITIO.
 *
 * Lo leen la sección de precios de la portada y el `Offer` de los datos
 * estructurados. Estaban separados y decían cosas distintas: la página anunciaba
 * el precio por usuario y el JSON-LD declaraba `price: '0'` —venía de la frase
 * «prueba gratuita sin tarjeta», que describe la PRUEBA y no el producto—. Un
 * buscador lee lo segundo, así que la suite salía anunciada como gratis, y esa
 * clase de discrepancia entre lo que se ve y lo que se declara es justo lo que
 * penalizan los resultados enriquecidos.
 */
export const PRECIO_SUITE = { importe: 5, moneda: 'EUR' } as const;

/**
 * La oferta de la suite en schema.org.
 *
 * `UnitPriceSpecification` y no un `price` a secas: «5 €» solo se entiende con
 * «por usuario y mes» al lado, y en un dato estructurado eso se dice con
 * `unitText` (por usuario) y `referenceQuantity` en meses (`unitCode: MON`). Con
 * `price` pelado, un agregador entiende que la suite entera cuesta cinco euros.
 *
 * `valueAddedTaxIncluded: false` porque el precio es sin impuestos
 * (docs/business.md), y no decirlo deja al lector suponiendo.
 */
export function ofertaSuite(t: T): Record<string, unknown> {
  return {
    '@type': 'Offer',
    priceCurrency: PRECIO_SUITE.moneda,
    availability: 'https://schema.org/InStock',
    description: t('seo.offer.description'),
    priceSpecification: {
      '@type': 'UnitPriceSpecification',
      price: PRECIO_SUITE.importe.toFixed(2),
      priceCurrency: PRECIO_SUITE.moneda,
      unitText: 'user',
      valueAddedTaxIncluded: false,
      referenceQuantity: {
        '@type': 'QuantitativeValue',
        value: 1,
        unitCode: 'MON',
      },
    },
  };
}

export function softwareApplicationJsonLd(t: T): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    '@id': `${SITE_ORIGIN}/#software`,
    name: 'Sytue One',
    applicationCategory: 'BusinessApplication',
    applicationSubCategory: 'ProductivitySuite',
    operatingSystem: 'Web',
    url: SITE_ORIGIN,
    description: t('seo.software.description'),
    inLanguage: SITE_LANGUAGES,
    provider: { '@id': `${SITE_ORIGIN}/#organization` },
    offers: ofertaSuite(t),
    featureList: [
      t('seo.software.f1'),
      t('seo.software.f2'),
      t('seo.software.f3'),
      t('seo.software.f4'),
    ],
  };
}
