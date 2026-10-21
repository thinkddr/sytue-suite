/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Productos con ficha propia en el sitio público. El TEXTO ya no vive aquí: sale
 * del catálogo (`product.<key>.*`), que es lo que permite que la ficha exista en
 * los 26 idiomas. Lo que queda es la lista de claves, y sigue en su propio
 * módulo porque Astro ELEVA `getStaticPaths` por encima del frontmatter: una
 * `const` declarada en el script de la página caería en TDZ al generarse las
 * rutas. Los imports sí se elevan.
 */
import { esSoloDeOrganizaciones, type ProductKey } from '@sytue/config';
import { CATALOGO, type Disponibilidad } from '@sytue/ui';
import type { T } from '../i18n';

/**
 * Productos con página de marketing, en orden de presentación. Fija a la vez las rutas
 * `/product/<key>`, el orden de la rejilla de la portada y los enlaces del pie.
 *
 * QUÉ se anuncia lo decide el CATÁLOGO (`CATALOGO` de `@sytue/ui`, encargo v4 §14): esta
 * lista tiene que ser, en conjunto, la de los productos cuya `disponibilidad` no sea
 * `no-disponible`, y `product-content.test.ts` lo comprueba. Lo que esta lista decide por
 * su cuenta es el ORDEN, que es diseño del sitio y no del catálogo.
 *
 * `cloud` (Sytue DevX) VUELVE aquí el 2026-09-21 (encargo v4 §14, «recuperar la tarjeta
 * de Sytue DevX»). Vuelve con su estado dicho, no disfrazado de producto vendible: sigue
 * en `desarrollo` en el catálogo y por eso la tarjeta y la ficha llevan distintivo y la
 * ficha no ofrece «Abrir». Estuvo fuera desde el 2026-09-15 (commit 8907c2f9f), y
 * esconderlo resultó ser la peor de las dos opciones: el sitio no describía la suite real
 * y nada impedía que volviera anunciado como disponible.
 */
export const PRODUCT_KEYS = [
  'one',
  'mail',
  'drive',
  'docs',
  'sheets',
  'slides',
  'pdf',
  'calendar',
  'contacts',
  'notes',
  'photos',
  'rooms',
  'cast',
  'cloud',
  'console',
  'id',
  'erp',
  'pages',
  'bases',
] as const satisfies readonly ProductKey[];

/** Producto con ficha propia. Más estrecho que `ProductKey`: no todos la tienen. */
export type MarketingKey = (typeof PRODUCT_KEYS)[number];

/**
 * El segmento de URL de una ficha cuando NO es la clave del producto. Sytue Web
 * conserva la clave `pages` —es la del catálogo, la del token de color y la de sus
 * textos (`product.pages.*`)—, pero su dirección pública es `/product/web`.
 */
const SLUG_DE_FICHA: Partial<Record<MarketingKey, string>> = { pages: 'web' };

/** Segmento de la ficha de `key` en `/product/<segmento>`. */
export function slugDeFicha(key: MarketingKey): string {
  return SLUG_DE_FICHA[key] ?? key;
}

/** Ruta BASE (sin prefijo de idioma) de la ficha de `key`: `/product/mail`, `/product/web`. */
export function rutaDeFicha(key: MarketingKey): string {
  return `/product/${slugDeFicha(key)}`;
}

/**
 * Lo que se genera bajo `/product/`: una página por ficha y, por cada ficha que cambió
 * de dirección, otra en la anterior que solo redirige (`redirigeA`). El sitio es
 * estático, así que la dirección vieja no se rompe con una regla de servidor sino con
 * una página: `/product/pages` existió del 2026-09-29 al 2026-10-04.
 */
export function rutasDeFichas(): { slug: string; key: MarketingKey; redirigeA?: string }[] {
  return PRODUCT_KEYS.flatMap((key) => {
    const ficha = { slug: slugDeFicha(key), key };
    return ficha.slug === key ? [ficha] : [ficha, { slug: key, key, redirigeA: rutaDeFicha(key) }];
  });
}

export interface ProductInfo {
  tagline: string;
  /** Estándar abierto o base técnica sobre la que está construido. */
  standard: string;
  points: string[];
  /** Todo lo que hace el producto: la lista larga y honesta de la ficha. */
  features: string[];
}

/**
 * Cuántas funcionalidades enumera cada ficha. Fijo por producto porque las
 * claves del catálogo (`product.<key>.f1…fN`) tienen que existir en los 26
 * idiomas: la cifra es el contrato entre esta lista y los catálogos.
 */
export const FEATURE_COUNT: Record<MarketingKey, number> = {
  one: 8,
  mail: 12,
  drive: 10,
  docs: 11,
  sheets: 10,
  slides: 8,
  pdf: 10,
  calendar: 11,
  contacts: 10,
  notes: 12,
  photos: 12,
  rooms: 13,
  cast: 11,
  cloud: 12,
  console: 12,
  id: 10,
  erp: 10,
  pages: 10,
  bases: 6,
};

/** Contenido de una ficha en el idioma de `t`. Copy honesto: solo lo que la suite ya hace. */
export function productInfo(t: T, key: MarketingKey): ProductInfo {
  return {
    tagline: t(`product.${key}.tagline`),
    standard: t(`product.${key}.standard`),
    points: [
      t(`product.${key}.p1`),
      t(`product.${key}.p2`),
      t(`product.${key}.p3`),
      t(`product.${key}.p4`),
    ],
    features: Array.from({ length: FEATURE_COUNT[key] }, (_, i) =>
      t(`product.${key}.f${i + 1}` as Parameters<T>[0]),
    ),
  };
}

/**
 * Productos que NO entran en una cuenta personal, pague o no: son de
 * organizaciones. Es la regla del ADR 0038 —Sytue ERP administra una
 * organización; Sytue Admin sí se abre a una persona para sus integraciones y
 * migraciones— y la de Sytue Web (decisión 17 del dueño). La lista es la de
 * `@sytue/config`, la misma que apaga la tesela en el conmutador y en el lanzador:
 * una copia aquí dejó de estar al día en cuanto hubo un segundo producto.
 */
export function soloOrganizaciones(key: string): boolean {
  return esSoloDeOrganizaciones(key);
}

/**
 * Docus, Calcus, Diapos y CertiPDF montan la MISMA ofimática (`@sytue/office`)
 * y comparten un dato que merece decirse una sola vez con la misma frase: su
 * motor de edición está escrito desde cero, no es una integración de terceros.
 * El resto de la suite no lleva esta línea porque no es lo que la distingue —
 * ahí lo que importa es el estándar abierto (`standard`), que ya tiene su sitio.
 */
export const FROM_SCRATCH_KEYS = ['docs', 'sheets', 'slides', 'pdf'] as const;

export function esMotorPropio(key: string): boolean {
  return (FROM_SCRATCH_KEYS as readonly string[]).includes(key);
}

/**
 * Disponibilidad REAL del producto, tal y como la declara el catálogo canónico. El sitio
 * público NO tiene una segunda opinión sobre esto: si el catálogo dice `desarrollo`, la
 * web lo dice también (encargo v4 §14, «diferenciar disponible, piloto, beta, en
 * desarrollo y no disponible» y «no inventar estados»).
 *
 * `?? 'disponible'` solo por el tipo: `PRODUCT_KEYS` sale del catálogo y lo ata un test.
 */
export function disponibilidadDe(key: MarketingKey): Disponibilidad {
  return CATALOGO.get(key)?.disponibilidad ?? 'disponible';
}

/**
 * Etiqueta de cada estado que el catálogo declara HOY. Deliberadamente INCOMPLETA: no
 * hay ningún producto en `piloto`, así que no hay una palabra «Piloto» adivinada en 26
 * idiomas esperando a que alguien la use. El día que la haya, `product-content.test.ts`
 * se pone en rojo pidiéndola, que es más barato que veintiséis traducciones muertas.
 *
 * «Beta» no pasa por el catálogo de idiomas: es la misma palabra en los 26.
 */
const ETIQUETA_ESTADO: Partial<Record<Disponibilidad, (t: T) => string>> = {
  desarrollo: (t) => t('grid.enDesarrollo'),
  beta: () => 'Beta',
};

/**
 * Distintivo del estado, o `null` si el producto está disponible y no hay nada que
 * advertir. Es la ÚNICA función que traduce estado → etiqueta: la usan la rejilla de la
 * portada y la ficha, para que digan lo mismo.
 */
export function etiquetaEstado(t: T, key: MarketingKey): string | null {
  const d = disponibilidadDe(key);
  return d === 'disponible' ? null : (ETIQUETA_ESTADO[d]?.(t) ?? null);
}

/** ¿Se puede entrar hoy en este producto? Decide si la ficha ofrece «Abrir». */
export function sePuedeAbrir(key: MarketingKey): boolean {
  const d = disponibilidadDe(key);
  return d === 'disponible' || d === 'piloto' || d === 'beta';
}

/**
 * Cuántas aplicaciones se abren HOY con una sola identidad. Es la cifra que la
 * portada pone en grande, y SALE DEL CATÁLOGO en vez de estar escrita a mano.
 *
 * Estaba escrita a mano, y decía **8** con catorce productos abiertos: la
 * portada llevaba meses anunciando la mitad de la suite que enseñaba tres
 * secciones más abajo. Una cifra inventada al lado de dos comprobables resta
 * credibilidad a las dos (encargo v4 §14, «no inventar contadores»).
 *
 * Se quedan fuera dos cosas, y las dos por el mismo motivo —la frase dice «con
 * una sola identidad», o sea aplicaciones en las que se ENTRA—: Sytue ID, que es
 * la identidad y no una aplicación detrás de ella, y lo que todavía no se puede
 * abrir (`sePuedeAbrir`), que hoy es Sytue DevX.
 */
export const APPS_CON_IDENTIDAD: number = PRODUCT_KEYS.filter(
  (k) => k !== 'id' && sePuedeAbrir(k),
).length;

/**
 * Nombre comercial en la web. El catálogo llama al ERP «ERP» a secas porque
 * en la barra de las apps compite con «Mail» o «Drive»; en la web se presenta
 * con la marca delante, como el resto de productos.
 */
export function nombreEnWeb(app: { key: string; name: string }): string {
  return app.key === 'erp' ? 'Sytue ERP' : app.name;
}
