// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';
import { sytueOffice } from '@sytue/office/astro';

/**
 * Sytue Diapos — presentaciones (diapos.sytue.eu/diapos). App propia del producto (decisión del
 * dueño del 2026-10-12): el editor, el BFF y las vistas son los de la ofimática
 * común (`@sytue/office`) y esto es su montaje. SSR para el BFF de sesión. Puerto
 * dev 4340 (docs/CONVENTIONS.md §6).
 */
export default defineConfig({
  site: 'https://diapos.sytue.eu',
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: la dirección canónica es
  // la de la ORGANIZACIÓN, https://<slug>.sytue.eu/diapos. Su subdominio propio
  // sirve el MISMO prefijo y redirige desde la raíz — las dos son válidas.
  //
  // Se hornea al construir (Astro no lo lee en caliente), y decide la URL de
  // los assets del HTML: por eso una imagen sirve UN prefijo y no la raíz.
  base: '/diapos',
  // Sin esto, Astro descarta el Host real y `Astro.url` acaba en
  // localhost: checkOrigin rechaza entonces todo POST con 403 (ver
  // @sytue/config/astro-security).
  security: sytueSecurity,
  adapter: node({ mode: 'standalone' }),
  // `sytueOffice()` monta el BFF común de la ofimática (`/api/*`…); las pantallas
  // son las de `src/pages`, que montan las vistas de `@sytue/office`.
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), sytueOffice(), preact({ compat: true })],
  server: { port: 4340, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: {
      noExternal: [
        '@sytue/ui',
        '@sytue/shell',
        '@sytue/auth',
        '@sytue/api-clients',
        '@sytue/config',
        '@sytue/formula',
        '@sytue/i18n',
        '@sytue/office',
      ],
    },
  },
});
