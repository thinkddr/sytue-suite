/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL SITIO PÚBLICO BEBE DEL CATÁLOGO (encargo v4 §14).
 *
 * `PRODUCT_KEYS` decide a la vez las rutas `/product/<key>`, la rejilla de la portada y
 * los enlaces del pie. Hasta ahora era una lista suelta, y una lista suelta se olvida:
 * así se quedó el sitio anunciando productos con nombres viejos y sin la ficha de otros.
 *
 * El reparto es: el CATÁLOGO decide QUÉ productos se presentan y CON QUÉ ESTADO, y `www`
 * decide en qué ORDEN los enseña, que es una decisión de diseño del sitio y no del
 * catálogo. Este test ata lo primero y deja libre lo segundo.
 *
 * Lo que cambió el 2026-09-21: antes la regla era «lo que no está disponible no tiene
 * ficha», y el resultado fue que Sytue DevX desapareció del sitio entero. Esconder un
 * producto no es diferenciarlo: la regla ahora es que TODO lo que existe tiene ficha y
 * lo que no está disponible lo dice con un distintivo.
 */
import { readdirSync } from 'node:fs';
import { expect, test } from 'bun:test';
import { CATALOGO } from '@sytue/ui';
import { useT } from '../i18n';
import {
  APPS_CON_IDENTIDAD,
  FEATURE_COUNT,
  PRODUCT_KEYS,
  disponibilidadDe,
  etiquetaEstado,
  productInfo,
  rutaDeFicha,
  rutasDeFichas,
  sePuedeAbrir,
} from './product-content.js';

/** Lo que el sitio público tiene que presentar: todo lo que existe, esté como esté. */
const presentables = [...CATALOGO.values()].filter((e) => e.disponibilidad !== 'no-disponible');

test('www presenta exactamente los productos del catálogo que existen', () => {
  // Por igualdad de CONJUNTOS, no de listas: el orden es cosa del sitio.
  //
  // Comparando como `string[]`: `PRODUCT_KEYS` es `MarketingKey[]` —más estrecho— y el
  // catálogo devuelve `ProductKey[]`, que incluye `chat`, `meet` y `hub`. Sin esto
  // `toEqual` no compila, y es la comparación la que tiene que ceder, no los tipos:
  // justamente lo que este test comprueba es que los dos conjuntos coincidan.
  const delSitio: string[] = [...PRODUCT_KEYS].sort();
  const delCatalogo: string[] = presentables.map((e) => e.key as string).sort();
  expect(delSitio).toEqual(delCatalogo);
});

test('Sytue DevX tiene ficha y se presenta como lo que es', () => {
  // La ficha volvió el 2026-09-21 (encargo v4 §14). Que esté no basta: tiene que
  // salir con su estado dicho y sin botón de «Abrir», porque cloud.sytue.eu no
  // responde. Si algún día DevX pasa a `disponible`, este test cambia de sentido solo:
  // el distintivo desaparece y el botón vuelve.
  expect(PRODUCT_KEYS).toContain('cloud');
  expect(disponibilidadDe('cloud')).toBe('desarrollo');
  expect(sePuedeAbrir('cloud')).toBe(false);
  expect(etiquetaEstado(useT('en'), 'cloud')).toBe('In development');
  // Y su ficha tiene TEXTO en los 26 idiomas, que es lo que costaba: sin las claves
  // `product.cloud.*` la página saldría con la cadena de la clave a la vista.
  for (const locale of ['en', 'es', 'fr', 'de', 'mt', 'ga'] as const) {
    const info = productInfo(useT(locale), 'cloud');
    expect([locale, info.tagline.startsWith('product.')]).toEqual([locale, false]);
    expect([locale, info.features.length]).toEqual([locale, FEATURE_COUNT.cloud]);
    expect([locale, info.features.some((f) => f.startsWith('product.'))]).toEqual([locale, false]);
  }
});

test('todo producto que no esté disponible lleva distintivo', () => {
  // El guardián de «diferenciar disponible, piloto, beta, en desarrollo y no
  // disponible»: si alguien pone un producto en `piloto`, aquí salta, porque
  // `ETIQUETA_ESTADO` todavía no tiene esa palabra en los 26 idiomas. Es más barato
  // que dejar veintiséis traducciones muertas esperando.
  const t = useT('en');
  for (const key of PRODUCT_KEYS) {
    const esperado = disponibilidadDe(key) === 'disponible' ? null : 'una etiqueta';
    expect([key, etiquetaEstado(t, key) === null ? null : 'una etiqueta']).toEqual([key, esperado]);
  }
});

test('la cifra de la portada la cuenta el catálogo, no una mano', () => {
  // La portada pone en grande «N aplicaciones con una sola identidad». Decía 8 con
  // catorce abiertas, porque el número estaba escrito en la plantilla.
  //
  // Se cuenta AQUÍ otra vez y desde `CATALOGO`, no desde `PRODUCT_KEYS`: un test que
  // repite la misma expresión de la implementación no comprueba nada, solo la copia.
  const abiertos = [...CATALOGO.values()].filter(
    (e) => e.key !== 'id' && ['disponible', 'piloto', 'beta'].includes(e.disponibilidad),
  );
  expect(APPS_CON_IDENTIDAD).toBe(abiertos.length);
  // Y que no vuelva a quedarse fija: si mañana se anuncia un producto más, este
  // número sube solo y el de la portada con él.
  expect(APPS_CON_IDENTIDAD).toBeGreaterThan(8);
});

test('ningún catálogo del sitio público usa un nombre retirado', async () => {
  /*
   * El guardián del renombrado v4. La tabla canónica (encargo §2) retira estos
   * nombres, y el sitio público es donde más caro sale que sobreviva uno: es lo
   * que lee quien todavía no es cliente.
   *
   * No es hipotético. Cuatro catálogos —et, fi, hr y hu— seguían diciendo
   * «certiDocs» en `product.drive.f6` mientras los otros veintidós decían
   * «CertiPDF»: el renombre se hizo a mano y se saltó justo los idiomas que nadie
   * relee. Un `grep` en 26 ficheros cuesta milisegundos y lo habría visto.
   *
   * Se mira el FICHERO entero, comentarios incluidos, a propósito: un catálogo de
   * idioma no lleva prosa, solo cadenas, así que no hay falsos positivos que
   * excusar — y si alguien deja el nombre viejo en un comentario, el siguiente que
   * traduzca lo copiará.
   */
  const RETIRADOS = [
    'certiDocs',
    'CertiDocs',
    'Sytue Cloud',
    'Sytue Console',
    'Sytue Portal',
    'Sytue Docs',
    'SyPhotos',
  ];
  const dir = new URL('../i18n/messages/', import.meta.url);
  const ficheros = readdirSync(dir).filter((f) => f.endsWith('.ts'));
  // Que los encuentre todos: si el glob falla, el test pasaría sin mirar nada.
  expect(ficheros.length).toBe(26);
  for (const f of ficheros) {
    const texto = await Bun.file(new URL(f, dir)).text();
    const encontrados = RETIRADOS.filter((viejo) => texto.includes(viejo));
    expect([f, encontrados]).toEqual([f, []]);
  }
});

test('cada ficha declara cuántas funciones enumera', () => {
  // `FEATURE_COUNT` es el contrato con los catálogos: `product.<key>.f1…fN` tienen que
  // existir en los 26 idiomas. Una ficha sin su cifra saldría con la lista vacía.
  for (const key of PRODUCT_KEYS) {
    expect([key, typeof FEATURE_COUNT[key]]).toEqual([key, 'number']);
    expect(FEATURE_COUNT[key]).toBeGreaterThan(0);
  }
});

test('la ficha de Sytue Web vive en /product/web y /product/pages solo redirige a ella', () => {
  // La clave del producto sigue siendo `pages`; lo que cambió es la dirección.
  expect(rutaDeFicha('pages')).toBe('/product/web');
  expect(rutaDeFicha('mail')).toBe('/product/mail');
  const rutas = rutasDeFichas();
  expect(rutas.find((r) => r.slug === 'web')).toEqual({ slug: 'web', key: 'pages' });
  expect(rutas.find((r) => r.slug === 'pages')).toEqual({
    slug: 'pages',
    key: 'pages',
    redirigeA: '/product/web',
  });
  // Ni un segmento repetido (dos páginas en la misma URL) ni una ficha sin página.
  expect(new Set(rutas.map((r) => r.slug)).size).toBe(rutas.length);
  expect(rutas.filter((r) => !r.redirigeA).map((r) => r.key)).toEqual([...PRODUCT_KEYS]);
});
