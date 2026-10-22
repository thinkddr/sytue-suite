// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import sitemap from '@astrojs/sitemap';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';

/**
 * Sytue One — sitio publico. Estatico (output static / SSG): la landing no
 * requiere sesion ni BFF; se sirve como HTML estatico con islas Preact puntuales
 * (client:visible/idle) para interactividad ligera. Tailwind v4 + tokens de
 * marca. Diseno moderno Sytue. Soberania 100% UE. docs/CONVENTIONS.md §0.8, §4.
 *
 * SEO (superficies publicas): SSG por defecto (TTFB minimo, CWV verdes),
 * canonical + hreflang (ingles BASE sin prefijo; el resto bajo su prefijo),
 * sitemap.xml via @astrojs/sitemap y robots.txt (public/robots.txt). Datos
 * estructurados JSON-LD y metadatos OpenGraph/Twitter viven en el layout.
 *
 * Rendimiento: islands con JS minimo, code-splitting, assets con hash e
 * inmutables (cache 1 ano), CSS inline por debajo del umbral, e imagenes
 * AVIF/WebP via el servicio de imagen de Astro (astro:assets, Sharp).
 */

/**
 * Los 26 idiomas de producto (docs/CONVENTIONS.md §13.1). El INGLES es la
 * base y va sin prefijo: `/` es una pagina real, no una redireccion, que es lo
 * que quieren tanto el rastreador como el `x-default`. El resto cuelgan de su
 * prefijo (`/es`, `/fr`, `/pl`…). Misma lista y etiquetas que `LOCALES` e
 * `intlTag` de @sytue/i18n: esto es un .mjs y no importa TypeScript.
 */
const LOCALES = [
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
const DEFAULT_LOCALE = 'en';

/** Etiqueta BCP-47 por idioma. `en-GB`, no `en-US`: somos una suite europea. */
const HREFLANG = {
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

export default defineConfig({
  site: 'https://sytue.eu',
  output: 'static',
  trailingSlash: 'never',

  // i18n: en por defecto (sin prefijo de ruta), el resto bajo /<locale>. Alimenta
  // hreflang (astro:i18n / @astrojs/sitemap) y las URLs canonicas alternas.
  i18n: {
    defaultLocale: DEFAULT_LOCALE,
    locales: LOCALES,
    routing: { prefixDefaultLocale: false, redirectToDefaultLocale: false },
  },

  /*
   * URLs espanolas de la etapa anterior (el sitio nacio con el espanol de base).
   * En build estatico Astro genera una pagina de redireccion por cada una, asi
   * que cualquier enlace que ya existiera fuera sigue llegando a su sitio en vez
   * de al 404. Los slugs nuevos van en INGLES en todos los idiomas (`/es/downloads`,
   * no `/es/descargas`): slugs localizados son mejor SEO, pero exigen una tabla
   * de rutas por idioma en cada pagina y en el selector, y hoy no compensan.
   */
  redirects: {
    '/en': '/',
    '/descargas': '/downloads',
    '/privacidad': '/privacy',
    '/terminos': '/terms',
    // Enumeradas y no `/producto/[key]` -> `/product/[key]`: la ficha vive ahora
    // en `[...locale]/product/[key]`, y un redirect dinamico exige que el destino
    // sea una ruta con los MISMOS parametros. Espejo de PRODUCT_KEYS
    // (src/lib/product-content.ts): un producto nuevo no necesita entrada aqui,
    // porque nunca tuvo URL en espanol.
    ...Object.fromEntries(
      ['mail', 'drive', 'docs', 'calendar', 'contacts', 'rooms', 'cloud', 'id'].map((key) => [
        `/producto/${key}`,
        `/product/${key}`,
      ]),
    ),
  },

  integrations: [
    sytueHealthEndpoint(),
    preact({ compat: true }),
    // sitemap.xml + sitemap-index.xml automaticos. Excluye superficies que no
    // deben indexarse (404). hreflang generado desde la config i18n.
    sitemap({
      i18n: {
        defaultLocale: DEFAULT_LOCALE,
        locales: HREFLANG,
      },
      // Ni el 404 ni `/product/pages`, la dirección anterior de la ficha de Sytue
      // Web, que solo redirige a `/product/web`.
      filter: (page) => !page.includes('/404') && !page.includes('/product/pages'),
      changefreq: 'weekly',
      priority: 0.7,
    }),
  ],

  // Optimizacion de imagenes: AVIF/WebP con Sharp (astro:assets). Las <Image>
  // del sitio se sirven en formatos modernos con dimensiones explicitas (evita
  // CLS) y lazy-loading por defecto.
  image: {
    // Solo se optimizan imagenes de dominios propios (soberania: sin terceros).
    domains: ['sytue.eu'],
    service: { entrypoint: 'astro/assets/services/sharp' },
  },

  server: { port: 4324, host: true },

  build: {
    // Un unico CSS por pagina, con inline del critico por debajo del umbral para
    // acelerar el LCP sin bloquear el render.
    inlineStylesheets: 'auto',
    assets: '_astro',
  },

  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui'] },
    build: {
      // Presupuesto de bundle: avisa si un chunk JS supera 100 kB (sin
      // comprimir). El sitio publico debe cargar JS minimo (islands).
      chunkSizeWarningLimit: 100,
      cssCodeSplit: true,
      // Nombres con hash de contenido -> cache inmutable de 1 ano en el runtime.
      rollupOptions: {
        output: {
          entryFileNames: '_astro/[name].[hash].js',
          chunkFileNames: '_astro/[name].[hash].js',
          assetFileNames: '_astro/[name].[hash][extname]',
        },
      },
    },
  },
});
